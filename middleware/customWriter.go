// add file in v.1.0.3
// customWriter.go is file that declare custom writer overriding default writer in gin context

package middleware

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
)

func GinHResponseWriter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer = &ginHResponseWriter{
			ResponseWriter: c.Writer,
		}
		c.Next()
	}
}

type ginHResponseWriter struct {
	gin.ResponseWriter
	json gin.H
}

// save response(value of gin.H type) in field of ginHResponseWriter
func (w *ginHResponseWriter) Write(b []byte) (i int, e error) {
	resp := gin.H{}
	if err := json.Unmarshal(b, &resp); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("error occurs while unmarshal response json to obj in custom response writer, err: %v\n", err)
		return
	}

	// status, code, message 필드 존재 여부 확인
	shouldContain := []string{"status", "code", "message"}
	for _, contain := range shouldContain {
		if _, ok := resp[contain]; !ok {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("json in response body have to contain %s, resp json: %v\n", contain, resp)
			return
		}
	}

	w.json = resp
	return w.ResponseWriter.Write(b)
}
