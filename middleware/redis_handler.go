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
	client   *redis.Client
	tracer   opentracing.Tracer
	setTopic string
	delTopic string
}

func RedisHandler(cli *redis.Client, tracer opentracing.Tracer, setTopic, delTopic string) *redisHandler {
	return &redisHandler{
		client:   cli,
		tracer:   tracer,
		setTopic: setTopic,
		delTopic: delTopic,
	}
}

func (r *redisHandler) ResponderAndSetEventPublisher(key string, successStatus int) []gin.HandlerFunc {
	return []gin.HandlerFunc{r.ResponderIfKeyExist(key), r.SetResponseEventPublisher(key, successStatus)}
}

// response value of redis key if exists instead request to service
func (r *redisHandler) ResponderIfKeyExist(key string) gin.HandlerFunc {
	if key == "" {
		systemlog.Fatalln("parameter of ResponderIfKeyExist to get redis key must not be blank string")
	}
	ctx := context.Background()

	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-Id")

		inAdvanceTopSpan, _ := c.Get("TopSpan")
		topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

		inAdvanceClaims, _ := c.Get("Claims")
		uuidClaims, _ := inAdvanceClaims.(jwtutil.UUIDClaims)

		inAdvanceReq, _ := c.Get("Request")
		reqBytes, _ := json.Marshal(inAdvanceReq)

		inAdvanceEntry, _ := c.Get("RequestLogEntry")
		entry, _ := inAdvanceEntry.(*logrus.Entry)

		redisSpan := r.tracer.StartSpan("GetRedis", opentracing.ChildOf(topSpan.Context())).SetTag("X-Request-Id", reqID)
		redisKey, err := r.formatKeyWithRequest(key, c, inAdvanceReq, uuidClaims)
		if redisKey == "" {
			if err == nil {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"status": http.StatusInternalServerError, "code": 0, "message": err.Error(),
			})
			entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": err.Error(), "request": string(reqBytes)}).Error()
			redisSpan.SetTag("success", false).LogFields(log.String("key", key), log.Error(err))
			redisSpan.Finish()
			return
		}

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

// publish set redis key event with request payload if success status
func (r *redisHandler) SetResponseEventPublisher(key string, successStatus int) gin.HandlerFunc {
	if key == "" {
		systemlog.Fatalln("parameter of SetResponseEventPublisher to set redis key must not be blank string")
	}
	ctx := context.Background()

	return func(c *gin.Context) {
		// run business logic handler
		c.Next()

		reqID := c.GetHeader("X-Request-Id")

		inAdvanceTopSpan, _ := c.Get("TopSpan")
		topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

		inAdvanceClaims, _ := c.Get("Claims")
		uuidClaims, _ := inAdvanceClaims.(jwtutil.UUIDClaims)

		inAdvanceReq, _ := c.Get("Request")

		redisSpan := r.tracer.StartSpan("PublishSetEvent", opentracing.ChildOf(topSpan.Context())).SetTag("X-Request-Id", reqID)
		status, resp := 0, gin.H{}
		switch w := c.Writer.(type) {
		case *ginHResponseWriter:
			status = w.status
			resp = w.json
		default:
			err := errors.New("unable to get response status code from default response writer")
			redisSpan.SetTag("success", false).LogFields(log.String("key", key), log.Error(err))
			redisSpan.Finish()
			return
		}

		if status != successStatus {
			err := errors.New("response status code is not success status code to set response in redis")
			redisSpan.SetTag("success", false).LogFields(log.String("key", key), log.Error(err))
			redisSpan.Finish()
			return
		}

		redisKey, err := r.formatKeyWithRequest(key, c, inAdvanceReq, uuidClaims)
		if err != nil {
			redisSpan.SetTag("success", false).LogFields(log.String("key", key), log.Error(err))
			redisSpan.Finish()
			return
		}

		resp["redis.key"] = redisKey
		respBytes, _ := json.Marshal(resp)
		result, err := r.client.Publish(ctx, r.setTopic, string(respBytes)).Result()

		if err != nil {
			redisSpan.SetTag("success", false)
		} else {
			redisSpan.SetTag("success", true)
		}
		redisSpan.LogFields(log.String("topic", r.setTopic), log.String("msg", string(respBytes)),
			log.String("key", redisKey), log.Int64("result", result), log.Error(err))
		redisSpan.Finish()
		return
	}
}

// publish delete redis key event with keys if success status
func (r *redisHandler) DeleteKeyEventPublisher(keys[] string, successStatus int) gin.HandlerFunc {
	for _, key := range keys {
		if key == "" {
			systemlog.Fatalln("parameter of DeleteKeyEventPublisher to delete redis key must not be blank string")
		}
	}
	ctx := context.Background()

	return func(c *gin.Context) {
		// run business logic handler
		c.Next()

		reqID := c.GetHeader("X-Request-Id")

		inAdvanceTopSpan, _ := c.Get("TopSpan")
		topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

		inAdvanceClaims, _ := c.Get("Claims")
		uuidClaims, _ := inAdvanceClaims.(jwtutil.UUIDClaims)

		inAdvanceReq, _ := c.Get("Request")

		redisSpan := r.tracer.StartSpan("PublishDeleteEvent", opentracing.ChildOf(topSpan.Context())).SetTag("X-Request-Id", reqID)
		var status int
		switch w := c.Writer.(type) {
		case *ginHResponseWriter:
			status = w.status
		default:
			err := errors.New("unable to get response status code from default response writer")
			redisSpan.SetTag("success", false).LogFields(log.Object("keys", keys), log.Error(err))
			redisSpan.Finish()
			return
		}

		if status != successStatus {
			err := errors.New("response status code is not success status code to delete key in redis")
			redisSpan.SetTag("success", false).LogFields(log.Object("keys", keys), log.Error(err))
			redisSpan.Finish()
			return
		}

		redisKeys := make([]string, len(keys))
		for i, key := range keys {
			redisKey, err := r.formatKeyWithRequest(key, c, inAdvanceReq, uuidClaims)
			if err != nil {
				redisSpan.SetTag("success", false).LogFields(log.String("key", redisKey), log.Error(err))
				redisSpan.Finish()
				return
			}
			redisKeys[i] = redisKey

			_, err = r.client.Publish(ctx, r.delTopic, redisKey).Result()
			if err != nil {
				redisSpan.SetTag("success", false).LogFields(log.String("topic", r.delTopic),
					log.String("key", redisKey), log.Error(err))
				redisSpan.Finish()
				return
			}
		}

		redisSpan.LogFields(log.String("topic", r.delTopic), log.Object("keys", redisKeys))
		redisSpan.Finish()
		return
	}
}

func (r *redisHandler) formatKeyWithRequest(key string, c *gin.Context, req interface{}, claims jwtutil.UUIDClaims) (redisKey string, err error) {
	var reqValue reflect.Value
	if req != nil {
		reqValue = reflect.ValueOf(req).Elem()
	}
	separatedKey := strings.Split(key, ".")
	formatted := make([]string, len(separatedKey))
	for i, sep := range separatedKey {
		if strings.HasPrefix(sep, "$") {
			param := strings.TrimPrefix(sep, "$")
			if (param == "student_uuid" || param == "writer_uuid") && c.Param(param) != claims.UUID {
				return
			}
			var paramValue string
			switch true {
			case c.Param(param) != "":
				paramValue = c.Param(param)
			case req != nil && reqValue.FieldByName(param).IsValid():
				switch field := reqValue.FieldByName(param); field.Interface().(type) {
				case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
					paramValue = strconv.Itoa(int(field.Int()))
				default:
					paramValue = field.String()
				}
			case param == "TokenUUID":
				paramValue = claims.UUID
			default:
				err = errors.New(fmt.Sprintf("unable to format param of redis key, key: %s, param: %s", key, param))
				return
			}
			formatted[i] = paramValue
		} else {
			formatted[i] = sep
		}
	}
	redisKey = strings.Join(formatted, ".")
	return
}
