// Add package in v.1.0.5
// subscriber package is used for handling event message occurred by SNS, RabbitMQ, etc ...
// you can start subscribe by calling Start method with parameter, specific signature function

package subscriber

import (
	"github.com/aws/aws-sdk-go/aws/session"
	log "github.com/micro/go-micro/v2/logger"
)

var awsSession *session.Session

func SetAwsSession(s *session.Session) {
	awsSession = s
}

type _default struct {
	awsSession  *session.Session
	listeners   []func()
	beforeStart []func()
}

type FieldSetter func(*_default)

func Default(setters ...FieldSetter) *_default {
	return newDefault(setters...)
}

func newDefault(setters ...FieldSetter) (h *_default) {
	h = new(_default)
	h.awsSession = awsSession
	for _, setter := range setters {
		setter(h)
	}
	h.listeners = []func(){}
	h.beforeStart = []func(){}
	return
}

func AwsSession(awsSession *session.Session) FieldSetter {
	return func(s *_default) {
		s.awsSession = awsSession
	}
}

// function that register listeners to run in StartListening method
func (s *_default) RegisterListeners(fn ...func()) {
	s.listeners = append(s.listeners, fn...)
}

func (s *_default) RegisterBeforeStart(fn ...func()) {
	s.beforeStart = append(s.beforeStart, fn...)
}

// function that start listening with listeners that register in RegisterListeners method
func (s *_default) StartListening() (_ error) {
	for _, before := range s.beforeStart {
		before()
	}
	
	log.Info("Default subscriber start listening!!")
	for _, listener := range s.listeners {
		go listener()
	}
	return
}
