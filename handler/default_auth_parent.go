package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gateway/entity"
	authproto "gateway/proto/golang/auth"
	jwtutil "gateway/tool/jwt"
	code "gateway/utils/code/golang"
	topic "gateway/utils/topic/golang"
	"github.com/dgrijalva/jwt-go"
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
	"reflect"
	"time"
)

func (h *_default) LoginParentAuth(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// logic handling BadRequest
	var receivedReq entity.LoginParentAuthRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
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

	var rpcResp *authproto.LoginParentAuthResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("LoginParentAuth", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.LoginParentAuth(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
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
			msg = fmt.Sprintf("request time out for LoginParentAuth service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("LoginParentAuth returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("LoginParentAuth returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to login parent auth"
		jwtToken, _ := jwtutil.GenerateStringWithClaims(jwtutil.UUIDClaims{
			UUID: rpcResp.LoggedInParentUUID,
			Type: "access_token",
			StandardClaims: jwt.StandardClaims{
				ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
			},
		}, jwt.SigningMethodHS512)
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "access_token": jwtToken, "parent_uuid": rpcResp.LoggedInParentUUID}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "login_uuid": rpcResp.LoggedInParentUUID,
			"response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) ChangeParentPW(c *gin.Context) {
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
	var receivedReq entity.ChangeParentPWRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
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

	var rpcResp *authproto.ChangeParentPWResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("ChangeParentPW", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ParentUUID = c.Param("parent_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.ChangeParentPW(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
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
			msg = fmt.Sprintf("request time out for ChangeParentPW service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("ChangeParentPW returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("ChangeParentPW returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusCreated:
		status, _code := http.StatusCreated, 0
		msg := fmt.Sprintf("succeed to change auth password of %s", uuidClaims.UUID)
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

func (h *_default) GetParentInformWithUUID(c *gin.Context) {
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

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
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

	var rpcResp *authproto.GetParentInformWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetParentInformWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(authproto.GetParentInformWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ParentUUID = c.Param("parent_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetParentInformWithUUID(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
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
			msg = fmt.Sprintf("request time out for GetParentInformWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetParentInformWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetParentInformWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := fmt.Sprintf("succeed to get parent inform, uuid: %s", uuidClaims.UUID)
		sendResp := gin.H{
			"status": status, "code": _code, "message": msg,
			"name": rpcResp.Name, "phone_number": rpcResp.PhoneNumber,
		}
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

func (h *_default) GetParentUUIDsWithInform(c *gin.Context) {
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
	var receivedReq entity.GetParentUUIDsWithInformRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok && !reflect.DeepEqual(receivedReq, entity.GetParentUUIDsWithInformRequest{}) {
	} else {
		if msg == "" { msg = "you must set up at least one parameter" }
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
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

	var rpcResp *authproto.GetParentUUIDsWithInformResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetParentUUIDsWithInform", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetParentUUIDsWithInform(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
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
			msg = fmt.Sprintf("request time out for GetParentUUIDsWithInform service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetParentUUIDsWithInform returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetParentUUIDsWithInform returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get parent uuid list with inform"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "parent_uuids": rpcResp.ParentUUIDs}
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

func (h *_default) GetChildrenInformsWithUUID(c *gin.Context) {
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

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.AuthServiceName)
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

	var rpcResp *authproto.GetChildrenInformsWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetChildrenInformsWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(authproto.GetChildrenInformsWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ParentUUID = c.Param("parent_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetChildrenInformsWithUUID(ctxForReq, rpcReq, callOpts...)
		authSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		authSrvSpan.Finish()
		return
	})

	if err == breaker.ErrBreakerOpen {
		msg := fmt.Sprintf("circuit breaker is open (service id: %s, time out: %s)", selectedNode.Id, h.BreakerCfg.Timeout.String())
		status, _code := http.StatusServiceUnavailable, code.CircuitBreakerOpen
		_ = h.consulAgent.FailTTLHealth(selectedNode.Metadata["CheckID"], breaker.ErrBreakerOpen.Error())
		time.AfterFunc(h.BreakerCfg.Timeout, func() { _ = h.consulAgent.PassTTLHealth(selectedNode.Metadata["CheckID"], "close circuit breaker") })
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
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
			msg = fmt.Sprintf("request time out for GetChildrenInformsWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetChildrenInformsWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetChildrenInformsWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get children informs list with parent uuid"
		children := make([]map[string]interface{}, len(rpcResp.ChildrenInform))
		for index, childInform := range rpcResp.ChildrenInform {
			children[index] = map[string]interface{}{
				"student_uuid":   childInform.StudentUUID,
				"grade":          childInform.Grade,
				"group":          childInform.Group,
				"student_number": childInform.StudentNumber,
				"name":           childInform.Name,
				"phone_number":   childInform.PhoneNumber,
				"profile_uri":    childInform.ImageURI,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "children": children}
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
