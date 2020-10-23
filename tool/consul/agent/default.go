package agent

import (
	"github.com/hashicorp/consul/api"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/registry"
)

type _default struct {
	Strategy selector.Strategy
	client   *api.Client
	next     selector.Next
	nodes    []*registry.Node
}

func Default(setters ...FieldSetter) *_default {
	return newDefault(setters...)
}

func newDefault(setters ...FieldSetter) (h *_default) {
	h = new(_default)
	for _, setter := range setters {
		setter(h)
	}
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
