package handler

import (
	"encoding/json"
	"fmt"
	"gateway/entity"
	jwtutil "gateway/tool/jwt"
	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/sirupsen/logrus"
	"net/http"
	"net/url"
	"time"
)

const (
	NaverOpenApiURI = "https://openapi.naver.com/v1/search/local.json"
)

func (h *_default) GetPlaceWithNaverOpenAPI(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// get token claim from middleware
	inAdvanceClaims, _ := c.Get("Claims")
	uuidClaims, _ := inAdvanceClaims.(jwtutil.UUIDClaims)
	entry = entry.WithField("user_uuid", uuidClaims.UUID)

	var limited bool
	h.mutex.Lock()
	if limitTableForNaver[uuidClaims.UUID] {
		limited = true
	} else {
		limitTableForNaver[uuidClaims.UUID] = true
		time.AfterFunc(time.Second * 5, func() {
			limitTableForNaver[uuidClaims.UUID] = false
		})
	}
	h.mutex.Unlock()

	if limited {
		msg := "you can use the API only once every 5 seconds, please wait"
		c.JSON(http.StatusLocked, gin.H{"status": http.StatusLocked, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusLocked, "code": 0, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.GetPlaceWithNaverOpenAPIRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	openApiSpan := h.tracer.StartSpan("GetPlaceWithNaverOpenAPI", opentracing.ChildOf(topSpan.Context()))
	openApiUri := fmt.Sprintf("%s?start=%d&display=%d&sort=%s&query=%s", NaverOpenApiURI, 1, 5, "comment", url.QueryEscape(receivedReq.Keyword))
	req, _ := http.NewRequest("GET", openApiUri, nil)
	req.Header.Set("X-Naver-Client-Id", naverClientID)
	req.Header.Set("X-Naver-Client-Secret", naverClientSecret)
	resp, err := h.client.Do(req)
	openApiSpan.SetTag("X-Request-Id", reqID).LogFields(log.String("user_uuid", uuidClaims.UUID),
		log.String("uri", openApiUri), log.Error(err), log.Object("response", resp))
	openApiSpan.Finish()

	if err != nil {
		status := http.StatusInternalServerError
		msg := "unexpected error occurs while sending request to naver open api"
		c.JSON(status, gin.H{"status": status, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": 0, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	decodedResp := new(entity.GetPlaceWithNaverOpenAPIResponse)
	if err := json.NewDecoder(resp.Body).Decode(decodedResp); err != nil {
		status := http.StatusInternalServerError
		msg := "unexpected error occurs while decoding response body from naver open api"
		c.JSON(status, gin.H{"status": status, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": 0, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("unexpected error occurs while decoding response body from naver open api, reason: %s", decodedResp.ErrorMessage)
		c.JSON(resp.StatusCode, gin.H{"status": resp.StatusCode, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": resp.StatusCode, "code": decodedResp.ErrorCode, "message": msg, "request": string(reqBytes)}).Warn()
		return
	}

	decodedResp.ErrorMessage = "succeed to get place list from naver open api"
	sendResp := gin.H{"status": resp.StatusCode, "code": 0, "message": decodedResp.ErrorMessage, "item": decodedResp.Items,
		"lastBuildDate": decodedResp.LastBuildDate, "total": decodedResp.Total, "start": decodedResp.Start, "display": decodedResp.Display}
	c.JSON(resp.StatusCode, sendResp)
	respBytes, _ := json.Marshal(sendResp)
	entry.WithFields(logrus.Fields{"status": resp.StatusCode, "code": decodedResp.ErrorCode, "message": decodedResp.ErrorMessage,
		"response": string(respBytes), "request": string(reqBytes), "date": time.Now().Format("2006-01-02")}).Info()

	return
}
