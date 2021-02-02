// add file in v.1.0.3
// customWriter.go is file that declare custom writer overriding default writer in gin context

package middleware

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"reflect"
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

	// status, code 필드 int 변환 가능 여부 확인 및 변환
	shouldIntType := []string{"status", "code"}
	for _, i := range shouldIntType {
		switch value := resp[i].(type) {
		case int:
		case int8:
			resp[i] = int(value)
		case int16:
			resp[i] = int(value)
		case int32:
			resp[i] = int(value)
		case int64:
			resp[i] = int(value)
		case uint:
			resp[i] = int(value)
		case uint8:
			resp[i] = int(value)
		case uint16:
			resp[i] = int(value)
		case uint32:
			resp[i] = int(value)
		case uint64:
			resp[i] = int(value)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("%s field should be converted into int type, current type: %s\n", i, reflect.TypeOf(value).String())
			return
		}
	}

	w.json = resp
	return w.ResponseWriter.Write(b)
}
