package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gateway/entity"
	clubproto "gateway/proto/golang/club"
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

func (h *_default) AddClubMember(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.AddClubMemberRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.AddClubMemberResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("AddClubMember", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.AddClubMember(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for AddClubMember service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("AddClubMember returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("AddClubMember returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to add new club member"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) DeleteClubMember(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.DeleteClubMemberResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("DeleteClubMember", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.DeleteClubMemberRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		rpcReq.StudentUUID = c.Param("student_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.DeleteClubMember(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for DeleteClubMember service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("DeleteClubMember returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("DeleteClubMember returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to delete club member"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Info()
	}

	return
}

func (h *_default) ChangeClubLeader(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.ChangeClubLeaderRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.ChangeClubLeaderResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("ChangeClubLeader", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.ChangeClubLeader(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for ChangeClubLeader service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("ChangeClubLeader returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("ChangeClubLeader returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to add new club member"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) ModifyClubInform(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.ModifyClubInformRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.ModifyClubInformResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("ModifyClubInform", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		callOpts := []client.CallOption{client.WithDialTimeout(time.Second * 2), client.WithRequestTimeout(time.Second * 6), client.WithAddress(selectedNode.Address)}
		rpcResp, rpcErr = h.clubService.ModifyClubInform(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for ModifyClubInform service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("ModifyClubInform returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("ModifyClubInform returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to modify club inform"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) DeleteClubWithUUID(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.DeleteClubWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("DeleteClubWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.DeleteClubWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.DeleteClubWithUUID(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for DeleteClubWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("DeleteClubWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("DeleteClubWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to delete club"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Info()
	}

	return
}

func (h *_default) RegisterRecruitment(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.RegisterRecruitmentRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.RegisterRecruitmentResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("RegisterRecruitment", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.RegisterRecruitment(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for RegisterRecruitment service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("RegisterRecruitment returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("RegisterRecruitment returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusCreated:
		status, _code := http.StatusCreated, 0
		msg := "succeed to register new recruitment"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "recruitment_uuid": rpcResp.RecruitmentUUID}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) ModifyRecruitment(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.ModifyRecruitmentRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.ModifyRecruitmentResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("ModifyRecruitment", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.RecruitmentUUID = c.Param("recruitment_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.ModifyRecruitment(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for ModifyRecruitment service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("ModifyRecruitment returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("ModifyRecruitment returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to change recruitment inform"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) DeleteRecruitment(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling Unauthorized
	var uuidClaims jwtutil.UUIDClaims
	if ok, claims, _code, msg := h.checkIfAuthenticated(c); ok {
		uuidClaims = claims
		entry = entry.WithField("user_uuid", uuidClaims.UUID)
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err != nil {
		status, _code, msg := h.getStatusCodeFromConsulErr(err)
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}
	entry = entry.WithField("SelectedNode", *selectedNode)

	h.mutex.Lock()
	if _, ok := h.breakers[selectedNode.Id]; !ok {
		h.breakers[selectedNode.Id] = breaker.New(h.BreakerCfg.ErrorThreshold, h.BreakerCfg.SuccessThreshold, h.BreakerCfg.Timeout)
	}
	h.mutex.Unlock()

	var rpcResp *clubproto.DeleteRecruitmentWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("DeleteRecruitmentWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.DeleteRecruitmentWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.RecruitmentUUID = c.Param("recruitment_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.DeleteRecruitmentWithUUID(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for DeleteRecruitmentWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("DeleteRecruitmentWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	default:
		status, _code, msg := 0, 0, ""
		switch rpcErr {
		case breaker.ErrBreakerOpen:
			status, _code = http.StatusServiceUnavailable, code.CircuitBreakerOpen
			msg = fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
			_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
			time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		default:
			status, _code = http.StatusInternalServerError, 0
			msg = fmt.Sprintf("DeleteRecruitmentWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to delete recruitment"
		sendResp := gin.H{"status": status, "code": _code, "message": msg}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Info()
	}

	return
}
