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

// delete all redis key with pattern sent from redis pub/sub
func (h *_default) DeleteRedisKeyWithPattern(msg *redis.Message) (err error) {
	keys, err := h.redisClient.Keys(ctx, msg.Payload).Result()
	if err != nil {
		err = errors.New(fmt.Sprintf("unable to execute redis KEYS cmd, err: %v", err))
	}

	for _, key := range keys {
		if _, err := h.redisClient.Del(ctx, key).Result(); err != nil {
			err = errors.New(fmt.Sprintf("unable to execute redis DEL cmd, key: %s, err: %v", key, err))
		}
	}

	log.Infof("delete all redis key with pattern!, msg: %s, key num: %d", msg.String(), len(keys))
	return
}
