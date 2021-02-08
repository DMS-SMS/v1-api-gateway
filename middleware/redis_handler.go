// add file in v.1.0.4
// redis_handler.go is file that declare various handler about redis etc searching, publishing, etc ...

package middleware

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"strings"
)

type redisHandler struct {
	client *redis.Client
	tracer opentracing.Tracer
}

func RedisHandler(cli *redis.Client, tracer opentracing.Tracer) *redisHandler {
	return &redisHandler{
		client: cli,
		tracer: tracer,
	}
}

// response value of redis key if exists instead request to service
func (r *redisHandler) ResponseIfExistWithKey(key string) gin.HandlerFunc {
	ctx := context.Background()
	separatedKey := strings.Split(key, ".")

	formatKey := func(c *gin.Context) string {
		formatted := make([]string, len(separatedKey))
		for i, sep := range separatedKey {
			if strings.HasPrefix(sep, "$") {
				formatted[i] = c.Param(strings.TrimPrefix(sep, "$"))
				continue
			}
			formatted[i] = sep
		}
		return strings.Join(formatted, ".")
	}

}
