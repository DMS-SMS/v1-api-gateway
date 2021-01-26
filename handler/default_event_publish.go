// add file in v.1.0.2
// default_event_publish.go is file that publish event from HTTP API to aws sns, rabbitMQ, etc ...

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

// method that publish consul change event to aws sns
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
