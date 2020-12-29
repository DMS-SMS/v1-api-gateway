package middleware

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type logEntrySetter struct {
	logger *logrus.Logger
}

func LogEntrySetter(l *logrus.Logger) gin.HandlerFunc {
	return (&logEntrySetter{
		logger: l,
	}).setLogEntry
}

// Log entry에 path, method, client_ip, X-Request-Id, header 정보 저장
func (l *logEntrySetter) setLogEntry(c *gin.Context) {
	headerBytes, _ := json.Marshal(c.Request.Header)

	entry := l.logger.WithFields(logrus.Fields{
		"path":         c.Request.URL.Path,
		"method":       c.Request.Method,
		"client_ip":    c.Request.RemoteAddr,
		"X-Request-Id": c.GetHeader("X-Request-Id"),
		"header":       string(headerBytes),
		"full_uri":     c.FullPath(),
	})
	c.Set("RequestLogEntry", entry)
}
