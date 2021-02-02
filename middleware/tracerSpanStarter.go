// add file in v.1.0.3
// tracerSpanStarter.go is file that gin middleware to start, log & end top span of tracer

package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
)

type tracerSpanStarter struct {
	tracer opentracing.Tracer
}

func TracerSpanStarter(t opentracing.Tracer) gin.HandlerFunc {
	return (&tracerSpanStarter{
		tracer: t,
	}).StartTracerSpan
}
