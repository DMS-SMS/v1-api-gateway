// add file in v.1.0.3
// customWriter.go is file that declare custom writer overriding default writer in gin context

package middleware

import (
	"github.com/gin-gonic/gin"
)

type ginHResponseWriter struct {
	gin.ResponseWriter
	json gin.H
}
