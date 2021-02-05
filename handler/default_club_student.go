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
	"strconv"
	"time"
)

func (h *_default) GetClubsSortByUpdateTime(c *gin.Context) {
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
	receivedReq, _ := inAdvanceReq.(*entity.GetClubsSortByUpdateTimeRequest)
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

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
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
			msg = fmt.Sprintf("GetClubsSortByUpdateTime returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
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
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) GetRecruitmentsSortByCreateTime(c *gin.Context) {
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
	receivedReq, _ := inAdvanceReq.(*entity.GetRecruitmentsSortByCreateTimeRequest)
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

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
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
			msg = fmt.Sprintf("GetRecruitmentsSortByCreateTime returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
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
					members[index]["grade"] = intGrade
				} else {
					members[index]["grade"] = member.Grade
				}
				if intNumber, err := strconv.Atoi(member.Number); err == nil {
					members[index]["number"] = intNumber
				} else {
					members[index]["number"] = member.Number
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
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) GetClubInformWithUUID(c *gin.Context) {
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

	switch rpcErr := err.(type) {
	case nil:
		break
	case *errors.Error:
		status, _code, msg := 0, 0, ""
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
			msg = fmt.Sprintf("GetClubInformWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
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
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message}).Info()
	}

	return
}

func (h *_default) GetClubInformsWithUUIDs(c *gin.Context) {
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
	receivedReq, _ := inAdvanceReq.(*entity.GetClubInformsWithUUIDsRequest)
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

	var rpcResp *clubproto.GetClubInformsWithUUIDsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetClubInformsWithUUIDs", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetClubInformsWithUUIDs(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetClubInformsWithUUIDs service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetClubInformsWithUUIDs returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetClubInformsWithUUIDs returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get club list with uuid list"
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
	case http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusServiceUnavailable:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Error()
	default:
		c.JSON(int(rpcResp.Status), gin.H{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message})
		entry.WithFields(logrus.Fields{"status": rpcResp.Status, "code": rpcResp.Code, "message": rpcResp.Message, "request": string(reqBytes)}).Info()
	}

	return
}

func (h *_default) GetRecruitmentInformWithUUID(c *gin.Context) {
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

	var rpcResp *clubproto.GetRecruitmentInformWithUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetRecruitmentInformWithUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetRecruitmentInformWithUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.RecruitmentUUID = c.Param("recruitment_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetRecruitmentInformWithUUID(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetRecruitmentInformWithUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetRecruitmentInformWithUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetRecruitmentInformWithUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get club inform with uuid"
		members := make([]map[string]interface{}, len(rpcResp.RecruitMembers))
		for index, member := range rpcResp.RecruitMembers {
			members[index] = map[string]interface{}{
				"field": member.Field,
			}
			if intGrade, err := strconv.Atoi(member.Grade); err == nil {
				members[index]["grade"] = intGrade
			} else {
				members[index]["grade"] = member.Grade
			}
			if intNumber, err := strconv.Atoi(member.Number); err == nil {
				members[index]["number"] = intNumber
			} else {
				members[index]["number"] = member.Number
			}
		}
		sendResp := gin.H{
			"status":           status,
			"code":             _code,
			"message":          msg,
			"recruitment_uuid": rpcResp.RecruitmentUUID,
			"club_uuid":        rpcResp.ClubUUID,
			"recruit_concept":  rpcResp.RecruitConcept,
			"recruit_members":  members,
			"start_period":     rpcResp.StartPeriod,
			"end_period":       rpcResp.EndPeriod,
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

func (h *_default) GetRecruitmentUUIDWithClubUUID(c *gin.Context) {
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

	var rpcResp *clubproto.GetRecruitmentUUIDWithClubUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetRecruitmentUUIDWithClubUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetRecruitmentUUIDWithClubUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.ClubUUID = c.Param("club_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetRecruitmentUUIDWithClubUUID(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetRecruitmentUUIDWithClubUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetRecruitmentUUIDWithClubUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetRecruitmentUUIDWithClubUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get uuid of recruitment in progress with club uuid"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "recruitment_uuid": rpcResp.RecruitmentUUID}
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

func (h *_default) GetRecruitmentUUIDsWithClubUUIDs(c *gin.Context) {
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
	receivedReq, _ := inAdvanceReq.(*entity.GetRecruitmentUUIDsWithClubUUIDsRequest)
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

	var rpcResp *clubproto.GetRecruitmentUUIDsWithClubUUIDsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetRecruitmentUUIDsWithClubUUIDs", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := receivedReq.GenerateGRPCRequest()
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetRecruitmentUUIDsWithClubUUIDs(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetRecruitmentUUIDsWithClubUUIDs service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetRecruitmentUUIDsWithClubUUIDs returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetRecruitmentUUIDsWithClubUUIDs returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg, "request": string(reqBytes)}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get recruitment uuid list with club list"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "uuids": rpcResp.RecruitmentUUIDs}
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

func (h *_default) GetAllClubFields(c *gin.Context) {
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

	var rpcResp *clubproto.GetAllClubFieldsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetAllClubFields", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetAllClubFieldsRequest)
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetAllClubFields(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetAllClubFields service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetAllClubFields returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetAllClubFields returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to all fields of club"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "fields": rpcResp.Fields}
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

func (h *_default) GetTotalCountOfClubs(c *gin.Context) {
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

	var rpcResp *clubproto.GetTotalCountOfClubsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetTotalCountOfClubs", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetTotalCountOfClubsRequest)
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetTotalCountOfClubs(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetTotalCountOfClubs service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetTotalCountOfClubs returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetTotalCountOfClubs returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get total count of clubs"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "count": rpcResp.Count}
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

func (h *_default) GetTotalCountOfCurrentRecruitments(c *gin.Context) {
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

	var rpcResp *clubproto.GetTotalCountOfCurrentRecruitmentsResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetTotalCountOfCurrentRecruitments", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetTotalCountOfCurrentRecruitmentsRequest)
		rpcReq.UUID = uuidClaims.UUID
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetTotalCountOfCurrentRecruitments(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetTotalCountOfCurrentRecruitments service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetTotalCountOfCurrentRecruitments returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetTotalCountOfCurrentRecruitments returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get total count of all recruitments in progress"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "count": rpcResp.Count}
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

func (h *_default) GetClubUUIDWithLeaderUUID(c *gin.Context) {
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

	var rpcResp *clubproto.GetClubUUIDWithLeaderUUIDResponse
	err = h.breakers[selectedNode.Id].Run(func() (rpcErr error) {
		authSrvSpan := h.tracer.StartSpan("GetClubUUIDWithLeaderUUID", opentracing.ChildOf(topSpan.Context()))
		ctxForReq := context.Background()
		ctxForReq = metadata.Set(ctxForReq, "X-Request-Id", reqID)
		ctxForReq = metadata.Set(ctxForReq, "Span-Context", authSrvSpan.Context().(jaeger.SpanContext).String())
		rpcReq := new(clubproto.GetClubUUIDWithLeaderUUIDRequest)
		rpcReq.UUID = uuidClaims.UUID
		rpcReq.LeaderUUID = c.Param("leader_uuid")
		callOpts := append(h.DefaultCallOpts, client.WithAddress(selectedNode.Address))
		rpcResp, rpcErr = h.clubService.GetClubUUIDWithLeaderUUID(ctxForReq, rpcReq, callOpts...)
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
			msg = fmt.Sprintf("request time out for GetClubUUIDWithLeaderUUID service, detail: %s", rpcErr.Detail)
			status, _code = http.StatusRequestTimeout, 0
		default:
			msg = fmt.Sprintf("GetClubUUIDWithLeaderUUID returns unexpected micro error, code: %d, detail: %s", rpcErr.Code, rpcErr.Detail)
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
			msg = fmt.Sprintf("GetClubUUIDWithLeaderUUID returns unexpected type of error, err: %s", rpcErr.Error())
		}
		c.JSON(status, gin.H{"status": status, "code": _code, "message": msg})
		entry.WithFields(logrus.Fields{"status": status, "code": _code, "message": msg}).Error()
		return
	}

	switch rpcResp.Status {
	case http.StatusOK:
		status, _code := http.StatusOK, 0
		msg := "succeed to get total count of all recruitments in progress"
		sendResp := gin.H{"status": status, "code": _code, "message": msg, "club_uuid": rpcResp.ClubUUID}
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
