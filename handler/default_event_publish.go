// add file in v.1.0.2
// default_event_publish.go is file that publish event from HTTP API to aws sns, rabbitMQ, etc ...

package handler

import (
	"context"
	"gateway/entity"
	announcementproto "gateway/proto/golang/announcement"
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
	outingproto "gateway/proto/golang/outing"
	scheduleproto "gateway/proto/golang/schedule"
	topic "gateway/utils/topic/golang"
	"github.com/gin-gonic/gin"
	"github.com/micro/go-micro/v2/client"
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

	//pubOutput, err := sns.New(h.awsSession).Publish(&sns.PublishInput{
	//	Message:  aws.String("ConsulChangeEvent"),
	//	TopicArn: aws.String(snsTopicArn),
	//})
	//if err != nil {
	//	c.JSON(http.StatusInternalServerError, err.Error())
	//	consulIndexMutex.Unlock()
	//	return
	//}

	_ = h.consulAgent.ChangeAllServiceNodes()
	h.publishConsulChangeEvent()

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

	log.Print("SUCCEED TO PUBLISH CONSUL CHANGE REQUEST TO ALL SERVICE!!")
	c.Status(http.StatusOK)
	return
}

// function that return closure publishing consul change event
func (h *_default) ConsulChangeEventPublisher() func() error {
	return func() (err error) {
		h.publishConsulChangeEvent()
		return
	}
}

func (h *_default) publishConsulChangeEvent() {
	go func() {
		authNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
		if err != nil {
			return
		}
		authCallOpts := append(h.DefaultCallOpts, client.WithAddress(authNode.Address))
		_, _ = h.authService.ChangeAllServiceNodes(context.Background(), &authproto.Empty{}, authCallOpts...)
	}()

	go func() {
		clubNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
		if err != nil {
			return
		}
		clubCallOpts := append(h.DefaultCallOpts, client.WithAddress(clubNode.Address))
		_, _ = h.clubService.ChangeAllServiceNodes(context.Background(), &clubproto.Empty{}, clubCallOpts...)
	}()

	go func() {
		outingNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
		if err != nil {
			return
		}
		outingCallOpts := append(h.DefaultCallOpts, client.WithAddress(outingNode.Address))
		_, _ = h.outingService.ChangeAllServiceNodes(context.Background(), &outingproto.Empty{}, outingCallOpts...)
	}()

	go func() {
		announcementNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
		if err != nil {
			return
		}
		announcementCallOpts := append(h.DefaultCallOpts, client.WithAddress(announcementNode.Address))
		_, _ = h.announcementService.ChangeAllServiceNodes(context.Background(), &announcementproto.Empty{}, announcementCallOpts...)
	}()

	go func() {
		scheduleNode, err := h.consulAgent.GetNextServiceNode(topic.ScheduleServiceName)
		if err != nil {
			return
		}
		scheduleCallOpts := append(h.DefaultCallOpts, client.WithAddress(scheduleNode.Address))
		_, _ = h.scheduleService.ChangeAllServiceNodes(context.Background(), &scheduleproto.Empty{}, scheduleCallOpts...)
	}()
}
