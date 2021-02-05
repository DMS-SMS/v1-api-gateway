// add package in v.1.0.2
// move directory from tool/consul/agent to consul/agent in v.1.0.2

package agent

import (
	"gateway/consul"
	"github.com/go-playground/validator/v10"
	"github.com/hashicorp/consul/api"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/registry"
	"sync"
)

type _default struct {
	Strategy selector.Strategy
	client   *api.Client
	//  next      selector.Next                    // before v.1.0.2
	//  nodes     []*registry.Node                 // before v.1.0.2
	next      map[consul.ServiceName]selector.Next    // change in v.1.0.2
	nodes     map[consul.ServiceName][]*registry.Node // change in v.1.0.2
	services  []consul.ServiceName                    // add in v.1.0.2
	nodeMutex sync.RWMutex                            // add in v.1.0.2
	validator *validator.Validate                     // add in v.1.0.3
}

func Default(setters ...FieldSetter) *_default {
	return newDefault(setters...)
}

func newDefault(setters ...FieldSetter) (h *_default) {
	h = new(_default)
	for _, setter := range setters {
		setter(h)
	}
	h.next = map[consul.ServiceName]selector.Next{}
	h.nodes = map[consul.ServiceName][]*registry.Node{}
	h.nodeMutex = sync.RWMutex{}
	h.validator = validator.New()
	return
}

type FieldSetter func(*_default)

func Client(c *api.Client) FieldSetter {
	return func(d *_default) {
		d.client = c
	}
}

func Strategy(s selector.Strategy) FieldSetter {
	return func(d *_default) {
		d.Strategy = s
	}
}

func Services(s []consul.ServiceName) FieldSetter {
	return func(d *_default) {
		d.services = s
	}
}
