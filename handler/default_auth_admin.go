package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
)

func (h *_default) CreateNewStudent(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// 로그에 필요한 정보
	// path, method, client_ip, X-Request-Id, header -> 미들웨어에서 처리
	// request, response, status, code, message -> 핸들러에서 처치

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  http.StatusInternalServerError,
			"message": "unable to get request log entry from middleware",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  http.StatusCreated,
		"message": "succeed to create new club",
	})
	return
}
