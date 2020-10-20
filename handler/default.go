package handler

import (
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
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
	logger     *logrus.Logger
	tracer     opentracing.Tracer
	validate   *validator.Validate
	breakers   map[string]*breaker.Breaker
	BreakerCfg BreakerConfig
	mutex      sync.Mutex
	// consul agent 인터페이스 추가 예정
}

type BreakerConfig struct {
	ErrorThreshold   int
	SuccessThreshold int
	Timeout          time.Duration
}
