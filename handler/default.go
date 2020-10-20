package handler

import (
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
	"gateway/tool/consul"
	"github.com/eapache/go-resiliency/breaker"
	"github.com/go-playground/validator/v10"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"sync"
	"time"
)

type _default struct {
	authService interface {
		authproto.AuthAdminService
		authproto.AuthStudentService
		authproto.AuthTeacherService
		authproto.AuthParentService
	}
	clubService interface {
		clubproto.ClubAdminService
		clubproto.ClubStudentService
		clubproto.ClubLeaderService
	}
	consulAgent consul.Agent
	logger      *logrus.Logger
	tracer      opentracing.Tracer
	breakers    map[string]*breaker.Breaker
	validate    *validator.Validate
	mutex       sync.Mutex
	BreakerCfg  BreakerConfig
}

type BreakerConfig struct {
	ErrorThreshold   int
	SuccessThreshold int
	Timeout          time.Duration
}

func Default(setters ...FieldSetter) (h *_default) {
	h = new(_default)

	for _, setter := range setters {
		setter(h)
	}

	h.BreakerCfg = BreakerConfig{
		ErrorThreshold:   3,
		SuccessThreshold: 3,
		Timeout:          time.Minute,
	}

	return
}

type FieldSetter func(*_default)
