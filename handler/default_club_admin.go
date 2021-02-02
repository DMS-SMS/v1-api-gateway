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

func (h *_default) CreateNewClub(c *gin.Context) {
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
	var receivedReq entity.CreateNewClubRequest
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

	var rpcResp *clubproto.CreateNewClubResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("CreateNewClub", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := []client.CallOption{client.WithDialTimeout(time.Second * 2), client.WithRequestTimeout(time.Second * 7), client.WithAddress(selectedNode.Address)}
		rpcResp, rpcErr = h.clubService.CreateNewClub(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for CreateNewClub service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("CreateNewClub returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("CreateNewClub returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusCreated:
		status, _code := http.StatusCreated, 0
		msg := "succeed to create new club"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "club_uuid": rpcResp.ClubUUID}
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
