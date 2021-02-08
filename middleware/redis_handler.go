// add file in v.1.0.4
// redis_handler.go is file that declare various handler about redis etc searching, publishing, etc ...

package middleware

import (
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
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
