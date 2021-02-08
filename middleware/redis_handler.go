// add file in v.1.0.4
// redis_handler.go is file that declare various handler about redis etc searching, publishing, etc ...

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/sirupsen/logrus"
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

	return func(c *gin.Context) {
		redisKey := formatKey(c)
		reqID := c.GetHeader("X-Request-Id")

		inAdvanceTopSpan, _ := c.Get("TopSpan")
		topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

		inAdvanceEntry, _ := c.Get("RequestLogEntry")
		entry, _ := inAdvanceEntry.(*logrus.Entry)

		inAdvanceReq, _ := c.Get("Request")

		redisSpan := r.tracer.StartSpan("GetRedis", opentracing.ChildOf(topSpan.Context())).SetTag("X-Request-Id", reqID)
		value, err := r.client.Get(ctx, redisKey).Result()
		if err != nil {
			err = errors.New(fmt.Sprintf("some error occurs while getting redis value with key, key: %s, err: %v", redisKey, err))
			redisSpan.SetTag("success", false).LogFields(log.String("key", redisKey), log.Error(err))
			redisSpan.Finish()
			c.Next()
			return
		}

		cashedResp := gin.H{}
		if err := json.Unmarshal([]byte(value), &cashedResp); err != nil {
			err = errors.New(fmt.Sprintf("some error occurs while unmarshaling value to gin.H, key: %s, value: %s, err: %v", redisKey, value, err))
			redisSpan.SetTag("success", false).LogFields(log.String("key", redisKey), log.String("value", value), log.Error(err))
			redisSpan.Finish()
			c.Next()
			return
		}
		respBytes, _ := json.Marshal(cashedResp)
		reqBytes, _ := json.Marshal(inAdvanceReq)

		redisSpan.SetTag("success", true).LogFields(log.String("key", redisKey), log.String("value", value))
		redisSpan.Finish()

		c.AbortWithStatusJSON(int(cashedResp["status"].(float64)), cashedResp)
		entry.WithFields(logrus.Fields{"status": cashedResp["status"], "code": cashedResp["code"], "message": cashedResp["message"],
			"response": string(respBytes), "request": string(reqBytes)}).Info()
	}
}
