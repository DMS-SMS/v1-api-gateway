package handler

import (
	"gateway/entity"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"sync"
	"time"
)

var consulIndexMutex = sync.Mutex{}

func (h *_default) PublishConsulChangeEvent (c *gin.Context) {
	respFor407 := struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status: http.StatusProxyAuthRequired,
		Message: "please send the request through the proxy",
	}

	switch true {
	case c.GetHeader(consulIndexHeader) != "":
	default:
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	// 이벤트 발생
	// 모든 서비스 조회 새로고침 (해당 서비스 포함)
	// 해당 서비스에 대한 연결을 새로 맺어야 한다는 뜻이니까 health checker도 받아서 ping 보냄
	// 참고로 해당 서비스가 새로 시작될 때도 이벤트 발생 필요. (없었을 수도 있으니)

	// 현재는 checks를 따로 처리하진 않음
	if c.GetHeader("Type") == "checks" {
		c.Status(http.StatusOK)
		return
	}

	service := serviceName(c.GetHeader("Service"))
	index := consulIndex(c.GetHeader(consulIndexHeader))

	consulIndexMutex.Lock()
	if _, exist := h.consulIndexFilter[service][index]; exist {
		c.Status(http.StatusConflict)
		consulIndexMutex.Unlock()
		return
	}

	var req []entity.PublishConsulChangeEventRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, err.Error())
		consulIndexMutex.Unlock()
		return
	}

	pubOutput, err := sns.New(h.awsSession).Publish(&sns.PublishInput{
		Message:  aws.String("ConsulChangeEvent"),
		TopicArn: aws.String(snsTopicArn),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, err.Error())
		consulIndexMutex.Unlock()
		return
	}

	if _, ok := h.consulIndexFilter[service]; !ok {
		h.consulIndexFilter[service] = map[consulIndex][]entity.PublishConsulChangeEventRequest{}
	}
	h.consulIndexFilter[service][index] = req
	time.AfterFunc(time.Minute, func() {
		consulIndexMutex.Lock()
		delete(h.consulIndexFilter[service], index)
		consulIndexMutex.Unlock()
	})
	consulIndexMutex.Unlock()

	log.Printf("PUBLISH NEW MESSAGE TO SNS! MESSAGE ID: %s, SNS ARN: %s\n", *pubOutput.MessageId, snsTopicArn)
	c.Status(http.StatusOK)
	return
}
