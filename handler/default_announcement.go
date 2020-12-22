package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gateway/entity"
	announcementproto "gateway/proto/golang/announcement"
	agenterrors "gateway/tool/consul/agent/errors"
	jwtutil "gateway/tool/jwt"
	code "gateway/utils/code/golang"
	topic "gateway/utils/topic/golang"
	"github.com/eapache/go-resiliency/breaker"
	"github.com/gin-gonic/gin"
	"github.com/micro/go-micro/v2/client"
	"github.com/micro/go-micro/v2/errors"
	"github.com/micro/go-micro/v2/metadata"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/sirupsen/logrus"
	"github.com/uber/jaeger-client-go"
	"net/http"
	"time"
)

func (h *_default) CreateAnnouncement(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.CreateAnnouncementRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", http.StatusBadRequest), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusBadRequest).SetTag("code", _code).Finish()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.DefaultAnnouncementResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("CreateAnnouncement", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.CreateAnnouncement(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for CreateAnnouncement service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("CreateAnnouncement returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("CreateAnnouncement returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusCreated:
		status, _code := http.StatusCreated, 0
		msg := "succeed to create new announcement"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "announcement_uuid": rpcResp.AnnouncementId}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) GetAnnouncements(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.GetAnnouncementsRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", http.StatusBadRequest), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusBadRequest).SetTag("code", _code).Finish()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.GetAnnouncementsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("GetAnnouncements", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.Type = c.Param("type")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.GetAnnouncements(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetAnnouncements service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetAnnouncements returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetAnnouncements returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get announcement list"
		announcements := make([]map[string]interface{}, len(rpcResp.Announcement))
		for index, announcement := range rpcResp.Announcement {
			announcements[index] = map[string]interface{}{
				"announcement_uuid": announcement.AnnouncementId,
				"number":            announcement.Number,
				"title":             announcement.Title,
				"date":              announcement.Date,
				"views":             announcement.Views,
				"writer_name":       announcement.WriterName,
				"is_checked":        announcement.IsChecked,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "announcements": announcements}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) GetAnnouncementDetail(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.GetAnnouncementDetailResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("GetAnnouncementDetail", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(announcementproto.GetAnnouncementDetailRequest)
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.AnnouncementId = c.Param("announcement_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.GetAnnouncementDetail(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetAnnouncementDetail service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetAnnouncementDetail returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetAnnouncementDetail returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get announcement detail inform with uuid"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "date": rpcResp.Date, "title": rpcResp.Title,
			"content": rpcResp.Content, "writer_name": rpcResp.WriterName, "target_grade": rpcResp.TargetGrade, "target_group": rpcResp.TargetGroup,
			"next_title": rpcResp.NextTitle, "next_announcement_uuid": rpcResp.NextAnnouncementId,
			"previous_title": rpcResp.PreviousTitle, "previous_announcement_uuid": rpcResp.PreviousAnnouncementId}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) UpdateAnnouncement(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.UpdateAnnouncementRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", http.StatusBadRequest), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusBadRequest).SetTag("code", _code).Finish()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.DefaultAnnouncementResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("UpdateAnnouncement", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.AnnouncementId = c.Param("announcement_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.UpdateAnnouncement(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for UpdateAnnouncement service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("UpdateAnnouncement returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("UpdateAnnouncement returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to update announcement"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "announcement_uuid": rpcResp.AnnouncementId}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) DeleteAnnouncement(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.DefaultAnnouncementResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("DeleteAnnouncement", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(announcementproto.DeleteAnnouncementRequest)
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.AnnouncementId = c.Param("announcement_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.DeleteAnnouncement(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for DeleteAnnouncement service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("DeleteAnnouncement returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("DeleteAnnouncement returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get announcement detail inform with uuid"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "announcement_uuid": rpcResp.AnnouncementId}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) CheckAnnouncement(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	if ok, claim, _code, msg := h.checkIfAuthenticated(c); ok {
		entry = entry.WithField("user_uuid", claim.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.CheckAnnouncementResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("CheckAnnouncement", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(announcementproto.CheckAnnouncementRequest)
		rpcReq.Uuid = c.Param("student_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.CheckAnnouncement(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for CheckAnnouncement service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("CheckAnnouncement returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("CheckAnnouncement returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get if non-check announcement is exist"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "club": rpcResp.Club, "school": rpcResp.School}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) SearchAnnouncements(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.SearchAnnouncementsRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", http.StatusBadRequest), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusBadRequest).SetTag("code", _code).Finish()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.GetAnnouncementsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("SearchAnnouncements", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.Type = c.Param("type")
		rpcReq.Query = c.Param("search_param")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.SearchAnnouncements(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for SearchAnnouncements service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("SearchAnnouncements returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("SearchAnnouncements returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get announcement list with query"
		announcements := make([]map[string]interface{}, len(rpcResp.Announcement))
		for index, announcement := range rpcResp.Announcement {
			announcements[index] = map[string]interface{}{
				"announcement_uuid": announcement.AnnouncementId,
				"number":            announcement.Number,
				"title":             announcement.Title,
				"date":              announcement.Date,
				"views":             announcement.Views,
				"writer_name":       announcement.WriterName,
				"is_checked":        announcement.IsChecked,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "announcements": announcements}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) GetMyAnnouncements(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")
	topSpan := h.tracer.StartSpan(fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())).SetTag("X-Request-Id", reqID)

	inAdvanceEntry, ok := c.Get("RequestLogEntry")
	entry, ok := inAdvanceEntry.(*logrus.Entry)
	if !ok {
		msg := "unable to get request log entry from middleware"
		c.JSON(http.StatusInternalServerError, gin.H{"status": http.StatusInternalServerError, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusInternalServerError, "code": 0, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", http.StatusInternalServerError), log.Int("code", 0), log.String("message", msg))
		topSpan.SetTag("status", http.StatusInternalServerError).SetTag("code", 0).Finish()
		return
	}

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.GetMyAnnouncementsRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", http.StatusBadRequest), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusBadRequest).SetTag("code", _code).Finish()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AnnouncementServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available announcement service node is not exist in consul"
		status, _code := http.StatusServiceUnavailable, code.AvailableServiceNotExist
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		msg := fmt.Sprintf("unable to get service node from consul agent, err: %s", err.Error())
		status, _code := http.StatusInternalServerError, 0
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *announcementproto.GetAnnouncementsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		announcementSrvSpan := h.tracer.StartSpan("GetMyAnnouncements", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", announcementSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = c.Param("writer_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.announcementService.GetMyAnnouncements(ctxForReq, rpcReq, callOpts...)
		announcementSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		announcementSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		var status, _code int
		var msg string
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetMyAnnouncements service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetMyAnnouncements returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetMyAnnouncements returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get announcement list with writer uuid"
		announcements := make([]map[string]interface{}, len(rpcResp.Announcement))
		for index, announcement := range rpcResp.Announcement {
			announcements[index] = map[string]interface{}{
				"announcement_uuid": announcement.AnnouncementId,
				"number":            announcement.Number,
				"title":             announcement.Title,
				"date":              announcement.Date,
				"views":             announcement.Views,
				"writer_name":       announcement.WriterName,
				"is_checked":        announcement.IsChecked,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "announcements": announcements}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Msg))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}
