package handler

import (
	"encoding/json"
	"gateway/entity"
	"gateway/tool/jwt"
	//code "gateway/utils/code/golang"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"net/http"
)

func (h *_default) CreateNewStudent(c *gin.Context) {
	//reqID := c.GetHeader("X-Request-Id")

	// 로그에 필요한 정보
	// path, method, client_ip, X-Request-Id, header -> 미들웨어에서 처리
	// request, response, status, code, message -> 핸들러에서 처치

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Warn()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwt.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var req entity.CreateNewStudentRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &req); ok {
	} else {
		reqBytes, _ := json.Marshal(req)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  http.StatusCreated,
		"message": "succeed to create new club",
	})
	return
}
