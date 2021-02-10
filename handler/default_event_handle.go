// add file in v.1.0.2
// default_event_handle.go is file declare method that handling event in _default struct about consul, etc ...

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	log "github.com/micro/go-micro/v2/logger"
	"regexp"
	"strings"
	"time"
)

var (
	ctx = context.Background()

	// check if payload contains {} for param
	paramStringRegex = regexp.MustCompile("{.*}")

	// outing regex
	studentOutingsRegex = regexp.MustCompile("^students.student-\\d{12}.outings$")
	allStudentsOutingsRegex = regexp.MustCompile("^students.\\*.outings$")
	outingsRegex = regexp.MustCompile("^outings$")
	outingRegex = regexp.MustCompile("^outings.outing-\\d{12}$")

	// schedule regex
	schedulesRegex = regexp.MustCompile("^schedules$")
	timetableRegex = regexp.MustCompile("^students.student-\\d{12}.timetable.years.\\d{4}.months.\\d{1,2}.days.\\d{1,2}$")

	studentUUIDRegex = regexp.MustCompile("^student-\\d{12}$")
)

func (h *_default) ChangeConsulNodes(message *sqs.Message) (err error) {
	err = h.consulAgent.ChangeAllServiceNodes()
	log.Infof("change all service nodes!, err: %v", err)
	return
}

// set response in redis key with response in message payload
func (h *_default) SetRedisKeyWithResponse(msg *redis.Message) (err error) {
	resp := gin.H{}
	if err = json.Unmarshal([]byte(msg.Payload), &resp); err != nil {
		err = errors.New(fmt.Sprintf("unable to unmarshal set redis key msg to golang struct, err: %v", err))
		return
	}

	if _, ok := resp["redis.key"]; !ok {
		err = errors.New("msg to set in redis have to include redis.key field")
		return
	}

	key := resp["redis.key"].(string)
	delete(resp, "redis.key")
	respBytes, _ := json.Marshal(resp)

	result, err := h.redisClient.Set(ctx, key, string(respBytes), time.Minute).Result()
	if err != nil {
		err = errors.New(fmt.Sprintf("unable to set response in redis key, err: %v", err))
		return
	}
	log.Infof("succeed to set response in redis key!, key: %s, result: %s", key, result)

	switch true {
	case outingRegex.MatchString(key):
		if _, ok := resp["student_uuid"]; !ok {
			return
		}
		sid, ok := resp["student_uuid"].(string)
		if !ok || !studentUUIDRegex.MatchString(sid) {
			return
		}
		key = fmt.Sprintf("%s.student_uuid", key)
		h.redisClient.Set(ctx, key, sid, 0)
	case timetableRegex.MatchString(key):
		h.redisClient.Set(ctx, key, string(respBytes), 0)
	}
	return
}

// delete all redis key associated with message payload using regexp
func (h *_default) DeleteAssociatedRedisKey(msg *redis.Message) (err error) {
	var payload, pattern = msg.Payload, ""
	payload = paramStringRegex.ReplaceAllStringFunc(payload, func(param string) string {
		param = strings.TrimSuffix(strings.TrimPrefix(param, "{"), "}")
		value, err := h.redisClient.Get(ctx, param).Result()
		if err != nil {
			return "*"
		}
		return value
	})

	switch true {
	case studentOutingsRegex.MatchString(payload):
		// ex) student.student-123412341234.outings -> "students.student-123412341234.outings.start.*.count.*"
		pattern = fmt.Sprintf("%s.start.*.count.*", payload)

	case allStudentsOutingsRegex.MatchString(payload):
		// ex) student.*.outings -> "students.*.outings.start.*.count.*"
		pattern = "students.*.outings.start.*.count.*"

	case outingsRegex.MatchString(payload):
		// ex) outings -> outings.start.*.count.*.status.*.grade.*.group.*.floor.*
		pattern = fmt.Sprintf("%s.start.*.count.*.status.*.grade.*.group.*.floor.*", payload)

	case outingRegex.MatchString(payload):
		// ex) outings.outing-123412341234 -> outings.outing-123412341234*
		pattern = fmt.Sprintf("%s*", payload)

	case schedulesRegex.MatchString(payload):
		// ex) schedules -> schedules*
		pattern = fmt.Sprintf("%s*", payload)
		
	default:
		err = errors.New(fmt.Sprintf("message does not match any regular expressions, msg payload: %s", payload))
		return
	}

	num, err := h.deleteRedisKeyWithPattern(pattern)
	if err != nil {
		err = errors.New(fmt.Sprintf("some error occurs while delete redis key with pattern, pattern: %s, err: %v", pattern, err))
		return
	}

	log.Infof("delete all redis key with pattern!, msg payload: %s pattern: %s, matched key num: %d", payload, pattern, num)
	return 
}

// delete all redis key with pattern sent from parameter
func (h *_default) deleteRedisKeyWithPattern(pattern string) (num int, err error) {
	keys, err := h.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		err = errors.New(fmt.Sprintf("unable to execute redis KEYS cmd, err: %v", err))
		return
	}
	num = len(keys)

	for _, key := range keys {
		if _, err = h.redisClient.Del(ctx, key).Result(); err != nil {
			err = errors.New(fmt.Sprintf("unable to execute redis DEL cmd, key: %s, err: %v", key, err))
			return
		}
	}

	return
}
