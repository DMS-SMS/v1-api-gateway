package handler

import (
	"gateway/consul"
	"gateway/entity"
	announcementproto "gateway/proto/golang/announcement"
	authproto "gateway/proto/golang/auth"
	clubproto "gateway/proto/golang/club"
	outingproto "gateway/proto/golang/outing"
	scheduleproto "gateway/proto/golang/schedule"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/eapache/go-resiliency/breaker"
	"github.com/go-playground/validator/v10"
	"github.com/go-redis/redis/v8"
	"github.com/micro/go-micro/v2/client"
	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"net/http"
	"sync"
	"time"
)

type serviceName string
type consulIndex string

type _default struct {
	authService authproto.AuthServiceClient
	clubService clubproto.ClubServiceClient
	outingService outingproto.OutingServiceClient
	scheduleService scheduleproto.ScheduleServiceClient
	announcementService announcementproto.AnnouncementServiceClient

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

	// filtering consul watch index per service (Add in v.1.0.2)
	consulIndexFilter map[serviceName]map[consulIndex][]entity.PublishConsulChangeEventRequest

	// aws session for publish message in sns (Add in v.1.0.2)
	awsSession *session.Session

	// redis client for cashing responses of services (Add in v.1.0.3)
	redisClient *redis.Client
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

func AuthService(authService authproto.AuthServiceClient) FieldSetter {
	return func(h *_default) {
		h.authService = authService
	}
}

func ClubService(clubService clubproto.ClubServiceClient) FieldSetter {
	return func(h *_default) {
		h.clubService = clubService
	}
}

func OutingService(outingService outingproto.OutingServiceClient) FieldSetter {
	return func(h *_default) {
		h.outingService = outingService
	}
}

func AnnouncementService(announcementService announcementproto.AnnouncementServiceClient) FieldSetter {
	return func(h *_default) {
		h.announcementService = announcementService
	}
}

func ScheduleService(scheduleService scheduleproto.ScheduleServiceClient) FieldSetter {
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

func AWSSession(awsSession *session.Session) FieldSetter {
	return func(h *_default) {
		h.awsSession = awsSession
	}
}

func RedisClient(r *redis.Client) FieldSetter {
	return func(h *_default) {
		h.redisClient = r
	}
}
