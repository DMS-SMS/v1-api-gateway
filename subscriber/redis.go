// create file in v.1.0.3
// redis.go is file that declare closure return method about listening redis message queue
// you can also use this by registry with method in subscriber

package subscriber

import (
	"github.com/go-redis/redis/v8"
)

// function signature type for redis message handler
type redisMsgHandler func(*redis.Message) error
