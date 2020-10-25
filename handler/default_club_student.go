package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"gateway/entity"
	clubproto "gateway/proto/golang/club"
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
	"strconv"
	"time"
)

func (h *_default) GetClubsSortByUpdateTime(c *gin.Context) {
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
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.GetClubsSortByUpdateTimeRequest
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
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available auth service node is not exist in consul"
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

	var rpcResp *clubproto.GetClubsSortByUpdateTimeResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetClubsSortByUpdateTime", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetClubsSortByUpdateTime(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetClubsSortByUpdateTime service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetClubsSortByUpdateTime returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetClubsSortByUpdateTime returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get clubs sort by update time"
		clubs := make([]map[string]interface{}, len(rpcResp.Informs))
		for index, clubInform := range rpcResp.Informs {
			clubs[index] = map[string]interface{}{
				"club_uuid":    clubInform.ClubUUID,
				"leader_uuid":  clubInform.LeaderUUID,
				"member_uuids": clubInform.MemberUUIDs,
				"name":         clubInform.Name,
				"location":     clubInform.Location,
				"field":        clubInform.Field,
				"link":         clubInform.Link,
				"introduction": clubInform.Introduction,
				"club_concept": clubInform.ClubConcept,
				"logo_uri":     clubInform.LogoURI,
			}
			if intField, err := strconv.Atoi(clubInform.Floor); err == nil {
				clubs[index]["floor"] = intField
			} else {
				clubs[index]["floor"] = clubInform.Floor
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "clubs": clubs}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Message))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Message))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) GetRecruitmentsSortByCreateTime(c *gin.Context) {
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
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	// logic handling BadRequest
	var receivedReq entity.GetRecruitmentsSortByCreateTimeRequest
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
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available auth service node is not exist in consul"
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

	var rpcResp *clubproto.GetRecruitmentsSortByCreateTimeResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetRecruitmentsSortByCreateTime", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetRecruitmentsSortByCreateTime(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetRecruitmentsSortByCreateTime service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetRecruitmentsSortByCreateTime returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetRecruitmentsSortByCreateTime returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get recruitments sort by create time"
		recruitments := make([]map[string]interface{}, len(rpcResp.Recruitments))
		for index, recruitment := range rpcResp.Recruitments {
			members := make([]map[string]interface{}, len(recruitment.RecruitMembers))
			for index, member := range recruitment.RecruitMembers {
				members[index] = map[string]interface{}{
					"field": member.Field,
				}
				if intGrade, err := strconv.Atoi(member.Grade); err == nil {
					recruitments[index]["grade"] = intGrade
				} else {
					recruitments[index]["grade"] = member.Grade
				}
				if intNumber, err := strconv.Atoi(member.Number); err == nil {
					recruitments[index]["number"] = intNumber
				} else {
					recruitments[index]["number"] = member.Number
				}
			}
			recruitments[index] = map[string]interface{}{
				"recruitment_uuid": recruitment.RecruitmentUUID,
				"club_uuid":        recruitment.ClubUUID,
				"recruit_concept":  recruitment.RecruitConcept,
				"recruit_members":  members,
				"start_period":     recruitment.StartPeriod,
				"end_period":       recruitment.EndPeriod,
			}
		}
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "recruitments": recruitments}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes), "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Message))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Message))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}

func (h *_default) GetClubInformWithUUID(c *gin.Context) {
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
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"status": http.StatusUnauthorized, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": http.StatusUnauthorized, "code": _code, "message": msg}).Info()
		topSpan.LogFields(log.Int("status", http.StatusUnauthorized), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", http.StatusUnauthorized).SetTag("code", _code).Finish()
		return
	}

	consulSpan := h.tracer.StartSpan("GetNextServiceNode", opentracing.ChildOf(topSpan.Context()))
	selectedNode, err := h.consulAgent.GetNextServiceNode(topic.ClubServiceName)
	if err == nil { consulSpan.SetTag("X-Request-Id", reqID).LogFields(log.Object("SelectedNode", *selectedNode)) }
	consulSpan.LogFields(log.Error(err))
	consulSpan.Finish()

	switch err {
	case nil:
		break
	case agenterrors.AvailableNodeNotExist:
		msg := "available auth service node is not exist in consul"
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

	var rpcResp *clubproto.GetClubInformWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetClubInformWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetClubInformWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetClubInformWithUUID(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetClubInformWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetClubInformWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
			status, _code = http.StatusInternalServerError, 0
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	default:
		status, _code := http.StatusInternalServerError, 0
		msg := fmt.Sprintf("GetClubInformWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get club inform with uuid"
		sendResp := gin.H{
			"status":       status,
			"code":         _code,
			"message":      msg,
			"club_uuid":    rpcResp.ClubUUID,
			"leader_uuid":  rpcResp.LeaderUUID,
			"member_uuids": rpcResp.MemberUUIDs,
			"name":         rpcResp.Name,
			"location":     rpcResp.Location,
			"field":        rpcResp.Field,
			"link":         rpcResp.Link,
			"introduction": rpcResp.Introduction,
			"club_concept": rpcResp.ClubConcept,
			"logo_uri":     rpcResp.LogoURI,
		}
		c.JSON(status, sendResp)
		respBytes, _ := json.Marshal(sendResp)
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "response": string(respBytes)}).Info()
		topSpan.LogFields(log.Int("status", status), log.Int("code", _code), log.String("message", msg))
		topSpan.SetTag("status", status).SetTag("code", _code).Finish()
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Error()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Message))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Info()
		topSpan.LogFields(log.Int("status", int(rpcResp.Status)), log.Int("code", int(rpcResp.Code)), log.String("message", rpcResp.Message))
		topSpan.SetTag("status", rpcResp.Status).SetTag("code", rpcResp.Code).Finish()
	}

	return
}
