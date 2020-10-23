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
