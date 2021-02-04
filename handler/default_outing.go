package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gateway/entity"
	outingproto "gateway/proto/golang/outing"
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

func (h *_default) CreateOuting(c *gin.Context) {
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

	// logic handling BadRequest
	var receivedReq entity.CreateOutingRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	var rpcResp *outingproto.CreateOutingResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		outingSrvSpan := h.tracer.StartSpan("CreateOuting", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.outingService.CreateOuting(ctxForReq, rpcReq, callOpts...)
		outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		outingSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for CreateOuting service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("CreateOuting returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("CreateOuting returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusCreated:
		status, _code := http.StatusCreated, 0
		msg := "succeed to create new outing"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "outing_uuid": rpcResp.OutingId}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) GetStudentOutings(c *gin.Context) {
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

	// logic handling BadRequest
	var receivedReq entity.GetStudentOutingsRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	var rpcResp *outingproto.GetStudentOutingsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		outingSrvSpan := h.tracer.StartSpan("GetStudentOutings", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.StudentId = c.Param("student_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.outingService.GetStudentOutings(ctxForReq, rpcReq, callOpts...)
		outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		outingSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetStudentOutings service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetStudentOutings returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetStudentOutings returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get outing informs with student uuid"
		outings := make([]map[string]interface{}, len(rpcResp.Outing))
		for index, outing := range rpcResp.Outing {
			outings[index] = map[string]interface{}{
				"outing_uuid":      outing.OutingId,
				"place":            outing.Place,
				"reason":           outing.Reason,
				"start_time":       outing.StartTime,
				"end_time":         outing.EndTime,
				"outing_situation": outing.Situation,
				"outing_status":    outing.Status,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "outings": outings}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()

		//byteArr, _ := json.Marshal(sendResp)
		//fmt.Println(string(byteArr))
		//test1 := gin.H{}
		//fmt.Println(json.Unmarshal(byteArr, &test1))
		//fmt.Println(test1)
		//redisSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
		//setResult := h.redisClient.Set(context.Background(), "test", string(byteArr), time.Minute)
		//redisSpan.SetTag("X-Request-Id", reqID).LogFields(log.String("ResultString", setResult.String()), log.Error(setResult.Err()))
		//redisSpan.Finish()
		//
		//getStr, err := h.redisClient.Get(context.Background(), "test").Result()
		//fmt.Println(getStr, err)
		//test := gin.H{}
		//fmt.Println(test)
		//fmt.Println(json.Unmarshal([]byte(getStr), &test))
		//fmt.Println(sendResp)
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) GetOutingInform(c *gin.Context) {
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

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	var rpcResp *outingproto.GetOutingInformResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		outingSrvSpan := h.tracer.StartSpan("GetOutingInform", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(outingproto.GetOutingInformRequest)
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.OutingId = c.Param("outing_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.outingService.GetOutingInform(ctxForReq, rpcReq, callOpts...)
		outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		outingSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetOutingInform service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetOutingInform returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetOutingInform returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get outing inform with outing uuid"
		sendResp := gin.H{
			"status":           status,
			"code":             _code,
			"message":          msg,
			"outing_uuid":      rpcResp.OutingId,
			"place":            rpcResp.Place,
			"reason":           rpcResp.Reason,
			"start_time":       rpcResp.StartTime,
			"end_time":         rpcResp.EndTime,
			"outing_situation": rpcResp.OutingSituation,
			"outing_status":    rpcResp.OutingStatus,
		}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
	}

	return
}

func (h *_default) GetCardAboutOuting(c *gin.Context) {
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

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	var rpcResp *outingproto.GetCardAboutOutingResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		outingSrvSpan := h.tracer.StartSpan("GetCardAboutOuting", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(outingproto.GetCardAboutOutingRequest)
		rpcReq.Uuid = uuidClaims.UUID
		rpcReq.OutingId = c.Param("outing_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.outingService.GetCardAboutOuting(ctxForReq, rpcReq, callOpts...)
		outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		outingSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetCardAboutOuting service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetCardAboutOuting returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetCardAboutOuting returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get card about outing with outing uuid"
		sendResp := gin.H{
			"status":        status,
			"code":          _code,
			"message":       msg,
			"place":         rpcResp.Place,
			"start_time":    rpcResp.StartTime,
			"end_time":      rpcResp.EndTime,
			"outing_status": rpcResp.OutingStatus,
			"name":          rpcResp.Name,
			"grade":         rpcResp.Grade,
			"group":         rpcResp.Group,
			"number":        rpcResp.Number,
			"profile_uri":   rpcResp.ProfileImageUri,
			"reason":        rpcResp.Reason,
		}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
	}

	return
}

func (h *_default) TakeActionInOuting(c *gin.Context) {
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
	} else if action := c.Param("action"); !(action == "parent-approve" || action == "parent-reject") {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		return
	}

	var methodName string
	switch c.Param("action") {
	case "start":
		methodName = "StartGoOut"
	case "end":
		methodName = "FinishGoOut"
	case "teacher-approve":
		methodName = "ApproveOuting"
	case "teacher-reject":
		methodName = "RejectOuting"
	case "certify":
		methodName = "CertifyOuting"
	case "parent-approve":
		methodName = "ApproveOutingByOCode"
	case "parent-reject":
		methodName = "RejectOutingByOCode"
	default:
		msg := "that action in uri is not supported"
		c.JSON(http.StatusNotFound, gin.H{"status": http.StatusNotFound, "code": 0, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusNotFound, "code": 0, "message": msg}).Info()
		return
	}

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	switch c.Param("action") {
	case "start", "end":
		var rpcResp *outingproto.GoOutResponse
		err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
			outingSrvSpan := h.tracer.StartSpan(methodName, opentracing.ChildOf(topSpan.Context()))
			ctxForReq := context.Background()
			ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
			ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
			rpcReq := new(outingproto.GoOutRequest)
			rpcReq.Uuid = uuidClaims.UUID
			rpcReq.OutingId = c.Param("outing_uuid")
			callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
			switch c.Param("action") {
			case "start":
				rpcResp, rpcErr = h.outingService.StartGoOut(ctxForReq, rpcReq, callOpts...)
			case "end":
				rpcResp, rpcErr = h.outingService.FinishGoOut(ctxForReq, rpcReq, callOpts...)
			}
			outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
			outingSrvSpan.Finish()
			return
		})

		switch rpcErr := err.(type) {
		case nil:
			break
		case *errors.Error:
			status, _code, msg := 0, 0, ""
			switch rpcErr.Code {
			case http.StatusRequestTimeout:
				msg = fmt.Sprintf("request time out for %s service, detail: %s", methodName, rpcErr.Detail)
				status, _code = http.StatusRequestTimeout, 0
			default:
				msg = fmt.Sprintf("%s returns unexpected micro error, code: %d, detail: %s", methodName, rpcErr.Code, rpcErr.Detail)
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
				msg = fmt.Sprintf("%s returns unexpected type of error, err: %s", methodName, rpcErr.Error())
			}
			c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
			return
		}

		switch rpcResp.Status {
		case http.StatusOK:
			status, _code := http.StatusOK, 0
			msg := "succeed to take action to outing"
			sendResp := gin.H{"status": status, "code": _code, "message": msg}
			c.JSON(status, sendResp)
			respBytes, _ := json.Marshal(sendResp)
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
			c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
			entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
		default:
			c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
			entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
		}

		return

	case "teacher-approve", "teacher-reject", "certify":
		var rpcResp *outingproto.ConfirmOutingResponse
		err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
			outingSrvSpan := h.tracer.StartSpan(methodName, opentracing.ChildOf(topSpan.Context()))
			ctxForReq := context.Background()
			ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
			ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
			rpcReq := new(outingproto.ConfirmOutingRequest)
			rpcReq.Uuid = uuidClaims.UUID
			rpcReq.OutingId = c.Param("outing_uuid")
			callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
			switch c.Param("action") {
			case "teacher-approve":
				rpcResp, rpcErr = h.outingService.ApproveOuting(ctxForReq, rpcReq, callOpts...)
			case "teacher-reject":
				rpcResp, rpcErr = h.outingService.RejectOuting(ctxForReq, rpcReq, callOpts...)
			case "certify":
				rpcResp, rpcErr = h.outingService.CertifyOuting(ctxForReq, rpcReq, callOpts...)
			}
			outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
			outingSrvSpan.Finish()
			return
		})

		switch rpcErr := err.(type) {
		case nil:
			break
		case *errors.Error:
			status, _code, msg := 0, 0, ""
			switch rpcErr.Code {
			case http.StatusRequestTimeout:
				msg = fmt.Sprintf("request time out for %s service, detail: %s", methodName, rpcErr.Detail)
				status, _code = http.StatusRequestTimeout, 0
			default:
				msg = fmt.Sprintf("%s returns unexpected micro error, code: %d, detail: %s", methodName, rpcErr.Code, rpcErr.Detail)
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
				msg = fmt.Sprintf("%s returns unexpected type of error, err: %s", methodName, rpcErr.Error())
			}
			c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
			return
		}

		switch rpcResp.Status {
		case http.StatusOK:
			status, _code := http.StatusOK, 0
			msg := "succeed to take action to outing"
			sendResp := gin.H{"status": status, "code": _code, "message": msg}
			c.JSON(status, sendResp)
			respBytes, _ := json.Marshal(sendResp)
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
			c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
			entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
		default:
			c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
			entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
		}

		return

	case "parent-approve", "parent-reject":
		var rpcResp *outingproto.ConfirmOutingByOCodeResponse
		err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
			outingSrvSpan := h.tracer.StartSpan(methodName, opentracing.ChildOf(topSpan.Context()))
			ctxForReq := context.Background()
			ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
			ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
			rpcReq := new(outingproto.ConfirmOutingByOCodeRequest)
			rpcReq.ConfirmCode = c.Request.URL.Query().Get("code")
			callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
			switch c.Param("action") {
			case "parent-approve":
				rpcResp, rpcErr = h.outingService.ApproveOutingByOCode(ctxForReq, rpcReq, callOpts...)
			case "parent-reject":
				rpcResp, rpcErr = h.outingService.RejectOutingByOCode(ctxForReq, rpcReq, callOpts...)
			}
			outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
			outingSrvSpan.Finish()
			return
		})

		switch rpcErr := err.(type) {
		case nil:
			break
		case *errors.Error:
			status, _code, msg := 0, 0, ""
			switch rpcErr.Code {
			case http.StatusRequestTimeout:
				msg = fmt.Sprintf("request time out for %s service, detail: %s", methodName, rpcErr.Detail)
				status, _code = http.StatusRequestTimeout, 0
			default:
				msg = fmt.Sprintf("%s returns unexpected micro error, code: %d, detail: %s", methodName, rpcErr.Code, rpcErr.Detail)
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
				msg = fmt.Sprintf("%s returns unexpected type of error, err: %s", methodName, rpcErr.Error())
			}
			c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
			return
		}

		switch rpcResp.Status {
		case http.StatusOK:
			status, _code := http.StatusOK, 0
			msg := "succeed to take action to outing"
			sendResp := gin.H{"status": status, "code": _code, "message": msg}
			c.JSON(status, sendResp)
			respBytes, _ := json.Marshal(sendResp)
			entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
			c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
			entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
		default:
			c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
			entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
		}

		return
	}
}

func (h *_default) GetOutingWithFilter(c *gin.Context) {
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

	// logic handling BadRequest
	var receivedReq entity.GetOutingWithFilterRequest
	if ok, _code, msg := h.checkIfValidRequest(c, &receivedReq); ok {
	} else {
		reqBytes, _ := json.Marshal(receivedReq)
		c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusBadRequest, "code": _code, "message": msg, "request": string(reqBytes)}).Info()
		return
	}
	reqBytes, _ := json.Marshal(receivedReq)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	var rpcResp *outingproto.OutingResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		outingSrvSpan := h.tracer.StartSpan("GetOutingWithFilter", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.Uuid = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.outingService.GetOutingWithFilter(ctxForReq, rpcReq, callOpts...)
		outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		outingSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetOutingWithFilter service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetOutingWithFilter returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetOutingWithFilter returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get outing informs with student uuid"
		outings := make([]map[string]interface{}, len(rpcResp.Outing))
		for index, outing := range rpcResp.Outing {
			outings[index] = map[string]interface{}{
				"outing_uuid":      outing.OutingId,
				"place":            outing.Place,
				"reason":           outing.Reason,
				"start_time":       outing.StartTime,
				"end_time":         outing.EndTime,
				"outing_situation": outing.Situation,
				"outing_status":    outing.Status,
				"name":             outing.Name,
				"grade":            outing.Grade,
				"group":            outing.Group,
				"number":           outing.Number,
				"is_late":          outing.IsLate,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "outings": outings}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) GetOutingByOCode(c *gin.Context) {
	reqID := c.GetHeader("X-Request-Id")

	// get top span from middleware
	inAdvanceTopSpan, _ := c.Get("TopSpan")
	topSpan, _ := inAdvanceTopSpan.(opentracing.Span)

	// get log entry from middleware
	inAdvanceEntry, _ := c.Get("RequestLogEntry")
	entry, _ := inAdvanceEntry.(*logrus.Entry)

	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.OutingServiceName)
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

	var rpcResp *outingproto.GetOutingByOCodeResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		outingSrvSpan := h.tracer.StartSpan("GetOutingByOCode", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", outingSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(outingproto.GetOutingByOCodeRequest)
		rpcReq.ConfirmCode = c.Param("OCode")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.outingService.GetOutingByOCode(ctxForReq, rpcReq, callOpts...)
		outingSrvSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("request", rpcReq), log.Object("response", rpcResp), log.Error(rpcErr))
		outingSrvSpan.Finish()
		return
	})

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
		switch rpcErr.Code {
		case http.StatusRequestTimeout:
			msg = fmt.Sprintf("request time out for GetOutingByOCode service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetOutingByOCode returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetOutingByOCode returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get outing inform with outing code"
		sendResp := gin.H{
			"status":           status,
			"code":             _code,
			"message":          msg,
			"outing_uuid":      rpcResp.OutingId,
			"place":            rpcResp.Place,
			"reason":           rpcResp.Reason,
			"start_time":       rpcResp.StartTime,
			"end_time":         rpcResp.EndTime,
			"outing_situation": rpcResp.Situation,
		}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Msg}).Info()
	}

	return
}
