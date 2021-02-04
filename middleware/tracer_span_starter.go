// add file in v.1.0.3
// tracerSpanStarter.go is file that gin middleware to start, log & end top span of tracer

package middleware

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	systemlog "log"
	"net/http"
)

type tracerSpanStarter struct {
	tracer opentracing.Tracer
}

func TracerSpanStarter(t opentracing.Tracer) gin.HandlerFunc {
	return (&tracerSpanStarter{
		tracer: t,
	}).startTracerSpan
}

// start, end top span of tracer & set log as response gotten by ResponseWriter
func (s *tracerSpanStarter) startTracerSpan(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := s.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)
	c.Set("TopSpan", topSpan)

	// run business logic handler
	c.Next()

	status, _code, msg := 0, 0, ""
	switch w := c.Writer.(type) {
	case *ginHResponseWriter:
		if !w.written {
			topSpan.LogFields(log.Object("response", w.json))
			topSpan.Finish()
			return
		}
		status = w.json["status"].(int)
		_code = w.json["code"].(int)
		msg = w.json["message"].(string)
	default:
		systemlog.Print("default ResponseWriter cannot get response in TracerSpanStarter middleware")
		c.Writer.WriteHeader(http.StatusInternalServerError)
		topSpan.Finish()
		return
	}

	topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
	topSpan.SetTag("status", status).SetTag("code", _code).Finish()
}
