// add file in v.1.0.4
// redis_handler.go is file that declare various handler about redis etc searching, publishing, etc ...

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	jwtutil "gateway/tool/jwt"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/sirupsen/logrus"
	systemlog "log"
	"net/http"
	"reflect"
	"strconv"
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
	if key == "" {
		systemlog.Fatalln("parameter of ResponseIfExistWithKey to set redis key must not be blank string")
	}
	ctx := context.Background()
	separatedKey := strings.Split(key, ".")

	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-Id")

		inAdvanceTopSpan, _ := c.Get("TopSpan")
		topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

		inAdvanceClaims, _ := c.Get("Claims")
		uuidClaims, _ := inAdvanceClaims.(jwtutil.UUIDClaims)

		inAdvanceReq, _ := c.Get("Request")
		reqValue := reflect.ValueOf(inAdvanceReq).Elem()
		reqBytes, _ := json.Marshal(inAdvanceReq)

		inAdvanceEntry, _ := c.Get("RequestLogEntry")
		entry, _ := inAdvanceEntry.(*logrus.Entry)

		formatted := make([]string, len(separatedKey))
		for i, sep := range separatedKey {
			if strings.HasPrefix(sep, "$") {
				param := strings.TrimPrefix(sep, "$")
				if param == "student_uuid" && c.Param(param) != uuidClaims.UUID {
					c.Next()
					return
				}
				var paramValue string
				switch true {
				case c.Param(param) != "":
					paramValue = c.Param(param)
				case reqValue.FieldByName(param).IsValid():
					switch field := reqValue.FieldByName(param); field.Interface().(type) {
					case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
						paramValue = strconv.Itoa(int(field.Int()))
					default:
						paramValue = field.String()
					}
				default:
					msg := fmt.Sprintf("unable to format param of redis key, key: %s, param: %s", key, param)
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"status": http.StatusInternalServerError, "code": 0, "message": msg,
					})
					entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg, "request": string(reqBytes)}).Error()
					return
				}
				formatted[i] = paramValue
			} else {
				formatted[i] = sep
			}
		}
		redisKey := strings.Join(formatted, ".")

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
		redisSpan.SetTag("success", true).LogFields(log.String("key", redisKey), log.String("value", value))
		redisSpan.Finish()

		c.AbortWithStatusJSON(int(cashedResp["status"].(float64)), cashedResp)
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
		entry.WithFields(logrus.Fields{"status": cashedResp["status"], "code": cashedResp["code"], "message": cashedResp["message"],
			"response": string(respBytes), "request": string(reqBytes)}).Info()
	}
}
