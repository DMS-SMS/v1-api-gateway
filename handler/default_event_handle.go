// add file in v.1.0.2
// default_event_handle.go is file declare method that handling event in _default struct about consul, etc ...

package handler

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-redis/redis/v8"
	log "github.com/micro/go-micro/v2/logger"
)

var ctx = context.Background()

func (h *_default) ChangeConsulNodes(message *sqs.Message) (err error) {
	err = h.consulAgent.ChangeAllServiceNodes()
	log.Infof("change all service nodes!, err: %v", err)
	return
}

func (h *_default) DeleteRedisKeyAssociatedWithResource(message *redis.Message) (err error) {
	return 
}

// delete all redis key with pattern sent from parameter
func (h *_default) deleteRedisKeyWithPattern(pattern string) (err error) {
	keys, err := h.redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		err = errors.New(fmt.Sprintf("unable to execute redis KEYS cmd, err: %v", err))
	}

	for _, key := range keys {
		if _, err := h.redisClient.Del(ctx, key).Result(); err != nil {
			err = errors.New(fmt.Sprintf("unable to execute redis DEL cmd, key: %s, err: %v", key, err))
		}
	}

	log.Infof("delete all redis key with pattern!, pattern: %s, matched key num: %d", pattern, len(keys))
	return
}

// sey new redis key with key name & value sent from redis pub/sub
//func (h *_default) SetNewRedisKey(msg *redis.Message) (err error) {
//
//}
