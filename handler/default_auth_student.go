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
	"time"
)

func (h *_default) LoginStudentAuth(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// get bound request entry from middleware
	inAdvanceReq, _ := c.Get("Request")
	receivedReq, _ := inAdvanceReq.(*entity.LoginStudentAuthRequest)
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

	var rpcResp *authproto.LoginStudentAuthResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("LoginStudentAuth", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.LoginStudentAuth(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for LoginStudentAuth service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("LoginStudentAuth returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("LoginStudentAuth returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to login student auth"
		jwtToken, _ := jwtutil.GenerateStringWithClaims(jwtutil.UUIDClaims{
			UUID: rpcResp.LoggedInStudentUUID,
			Type: "access_token",
			StandardClaims: jwt.StandardClaims{
				ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
			},
		}, jwt.SigningMethodHS512)
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "access_token": jwtToken, "student_uuid": rpcResp.LoggedInStudentUUID}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "login_uuid": rpcResp.LoggedInStudentUUID,
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

func (h *_default) ChangeStudentPW(c *gin.Context) {
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

	// get bound request entry from middleware
	inAdvanceReq, _ := c.Get("Request")
	receivedReq, _ := inAdvanceReq.(*entity.ChangeStudentPWRequest)
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

	var rpcResp *authproto.ChangeStudentPWResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("ChangeStudentPW", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.StudentUUID = c.Param("student_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.ChangeStudentPW(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for ChangeStudentPW service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("ChangeStudentPW returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("ChangeStudentPW returns unexpected type of error, err: %s", rpcErr.Error())
		}
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

func (h *_default) GetStudentInformWithUUID(c *gin.Context) {
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

	var rpcResp *authproto.GetStudentInformWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetStudentInformWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(authproto.GetStudentInformWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.StudentUUID = c.Param("student_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetStudentInformWithUUID(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetStudentInformWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetStudentInformWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetStudentInformWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := fmt.Sprintf("succeed to get student inform, uuid: %s", uuidClaims.UUID)
		sendResp := gin.H{
			"status": status, "code": _code, "message": msg,
			"name": rpcResp.Name, "phone_number": rpcResp.PhoneNumber, "profile_uri": rpcResp.ImageURI,
			"grade": rpcResp.Grade, "group": rpcResp.Group, "student_number": rpcResp.StudentNumber,
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

func (h *_default) GetStudentUUIDsWithInform(c *gin.Context) {
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

	// get bound request entry from middleware
	inAdvanceReq, _ := c.Get("Request")
	receivedReq, _ := inAdvanceReq.(*entity.GetStudentUUIDsWithInformRequest)
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

	var rpcResp *authproto.GetStudentUUIDsWithInformResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetStudentUUIDsWithInform", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetStudentUUIDsWithInform(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetStudentUUIDsWithInform service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetStudentUUIDsWithInform returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetStudentUUIDsWithInform returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get student uuid list with inform"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "student_uuids": rpcResp.StudentUUIDs}
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

func (h *_default) GetStudentInformsWithUUIDs(c *gin.Context) {
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

	// get bound request entry from middleware
	inAdvanceReq, _ := c.Get("Request")
	receivedReq, _ := inAdvanceReq.(*entity.GetStudentInformsWithUUIDsRequest)
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

	var rpcResp *authproto.GetStudentInformsWithUUIDsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetStudentInformsWithUUIDs", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetStudentInformsWithUUIDs(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetStudentInformsWithUUIDs service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetStudentInformsWithUUIDs returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetStudentInformsWithUUIDs returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get student informs list with uuid list"
		students := make([]map[string]interface{}, len(rpcResp.StudentInforms))
		for index, studentInform := range rpcResp.StudentInforms {
			students[index] = map[string]interface{}{
				"student_uuid":   studentInform.StudentUUID,
				"grade":          studentInform.Grade,
				"group":          studentInform.Group,
				"student_number": studentInform.StudentNumber,
				"name":           studentInform.Name,
				"phone_number":   studentInform.PhoneNumber,
				"profile_uri":    studentInform.ImageURI,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "students": students}
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

func (h *_default) GetParentWithStudentUUID(c *gin.Context) {
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

	var rpcResp *authproto.GetParentWithStudentUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetParentWithStudentUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(authproto.GetParentWithStudentUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.StudentUUID = c.Param("student_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetParentWithStudentUUID(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetParentWithStudentUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetParentWithStudentUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetParentWithStudentUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get parent inform with student uuid"
		sendResp := gin.H{
			"status": status, "code": _code, "message": msg,
			"parent_uuid": rpcResp.ParentUUID, "name": rpcResp.Name, "phone_number": rpcResp.PhoneNumber,
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

func (h *_default) GetUnsignedStudentWithAuthCode(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	// get bound request entry from middleware
	inAdvanceReq, _ := c.Get("Request")
	receivedReq, _ := inAdvanceReq.(*entity.GetUnsignedStudentWithAuthCodeRequest)
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

	var rpcResp *authproto.GetUnsignedStudentWithAuthCodeResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetStudentInformWithAuthCode", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.authService.GetUnsignedStudentWithAuthCode(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetStudentInformWithAuthCode service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetStudentInformWithAuthCode returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetStudentInformWithAuthCode returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		sendResp := gin.H{
			"status": status, "code": _code, "message": rpcResp.Message,
			"name": rpcResp.Name, "phone_number": rpcResp.PhoneNumber,
			"grade": rpcResp.Grade, "group": rpcResp.Group, "student_number": rpcResp.StudentNumber,
		}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": rpcResp.Message, "request": string(reqBytes), "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}
