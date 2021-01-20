package handler

import (
	"gateway/entity"
	announcementproto "gateway/proto/golang/announcement"
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
	outingproto "gateway/proto/golang/outing"
	scheduleproto "gateway/proto/golang/schedule"
	"gateway/tool/consul"
	"github.com/eapache/go-resiliency/breaker"
	"github.com/go-playground/validator/v10"
	"github.com/micro/go-micro/v2/client"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"net/http"
	"sync"
	"time"
)

type serviceName string
type consulIndex int

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
	outingService interface {
		outingproto.OutingStudentService
		outingproto.OutingTeacherService
		outingproto.OutingParentsService
	}
	scheduleService interface {
		scheduleproto.ScheduleService
	}
	announcementService interface {
		announcementproto.AnnouncementService
	}

	consulAgent     consul.Agent
	logger          *logrus.Logger
	tracer          opentracing.Tracer
	validate        *validator.Validate
	breakers        map[string]*breaker.Breaker
	mutex           sync.Mutex
	BreakerCfg      BreakerConfig
	DefaultCallOpts []client.CallOption
	client          *http.Client
	location        *time.Location

	// filtering consul watch index per service
	consulIndexFilter map[serviceName]map[consulIndex][]entity.PublishConsulChangeEventRequest
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
		ErrorThreshold:   5,
		SuccessThreshold: 5,
		Timeout:          time.Minute,
	}
	h.DefaultCallOpts = []client.CallOption{client.WithDialTimeout(time.Second * 2), client.WithRequestTimeout(time.Second * 3)}
	h.mutex = sync.Mutex{}
	h.breakers = map[string]*breaker.Breaker{}
	h.client = &http.Client{}
	h.consulIndexFilter = map[serviceName]map[consulIndex][]entity.PublishConsulChangeEventRequest{}

	return
}

type FieldSetter func(*_default)

func AuthService(authService struct {
	authproto.AuthAdminService
	authproto.AuthStudentService
	authproto.AuthTeacherService
	authproto.AuthParentService
}) FieldSetter {
	return func(h *_default) {
		h.authService = authService
	}
}

func ClubService(clubService struct {
	clubproto.ClubAdminService
	clubproto.ClubStudentService
	clubproto.ClubLeaderService
}) FieldSetter {
	return func(h *_default) {
		h.clubService = clubService
	}
}

func OutingService(outingService interface {
	outingproto.OutingStudentService
	outingproto.OutingTeacherService
	outingproto.OutingParentsService
}) FieldSetter {
	return func(h *_default) {
		h.outingService = outingService
	}
}

func AnnouncementService(announcementService interface {
	announcementproto.AnnouncementService
}) FieldSetter {
	return func(h *_default) {
		h.announcementService = announcementService
	}
}

func ScheduleService(scheduleService interface {
	scheduleproto.ScheduleService
}) FieldSetter {
	return func(h *_default) {
		h.scheduleService = scheduleService
	}
}

func ConsulAgent(consulAgent consul.Agent) FieldSetter {
	return func(h *_default) {
		h.consulAgent = consulAgent
	}
}

func Logger(logger *logrus.Logger) FieldSetter {
	return func(h *_default) {
		h.logger = logger
	}
}

func Tracer(tracer opentracing.Tracer) FieldSetter {
	return func(h *_default) {
		h.tracer = tracer
	}
}

func Validate(validate *validator.Validate) FieldSetter {
	return func(h *_default) {
		h.validate = validate
	}
}

func Location(location *time.Location) FieldSetter {
	return func(h *_default) {
		h.location = location
	}
}
