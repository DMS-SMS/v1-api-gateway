// create file in v.1.0.4
// redis.go is file that declare closure return method about listening redis message queue
// you can also use this by registry with method in subscriber

package subscriber

import (
	"context"
	"github.com/go-redis/redis/v8"
	log "github.com/micro/go-micro/v2/logger"
)

// function signature type for redis message handler
type redisMsgHandler func(*redis.Message) error

// function that returns closure listening redis message & handling with function receive from parameter
func RedisListener(topic string, handler redisMsgHandler, chlSize int) func() {
	pubsub := redisCli.Subscribe(context.Background(), topic)
	pubChl := pubsub.ChannelSize(chlSize)

	return func() {
		for {
			pubMsg := <- pubChl
			go func(msg *redis.Message) {
				if err := handler(msg); err != nil {
					log.Errorf("some error occurs while handling redis message, topic: %s, err: %v", topic, err)
				}
			}(pubMsg)
		}
	}
}
