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
	"time"
)

var (
	ctx = context.Background()

	studentOutingsRegex = regexp.MustCompile("^students.student-\\d{12}.outings$")
	allStudentsOutingsRegex = regexp.MustCompile("^students.\\*.outings$")
	allOutingsRegex = regexp.MustCompile("^outings$")
	outingRegex = regexp.MustCompile("^outings.outing-\\d{12}$")
)

func (h *_default) ChangeConsulNodes(message *sqs.Message) (err error) {
	err = h.consulAgent.ChangeAllServiceNodes()
	log.Infof("change all service nodes!, err: %v", err)
	return
}

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
	return
}

func (h *_default) DeleteAssociatedRedisKey(message *redis.Message) (err error) {
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

	log.Infof("delete all redis key with pattern!, pattern: %s, matched key num: %d", pattern, len(keys))
	return
}

// sey new redis key with key name & value sent from redis pub/sub
//func (h *_default) SetNewRedisKey(msg *redis.Message) (err error) {
//
//}
