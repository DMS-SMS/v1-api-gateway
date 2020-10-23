package agent

import (
	"errors"
	"fmt"
	consulerrors "gateway/tool/consul/agent/errors"
	"github.com/hashicorp/consul/api"
	"github.com/micro/go-micro/v2/client/selector"
	"github.com/micro/go-micro/v2/registry"
	"reflect"
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

func (d *_default) GetNextServiceNode(service string) (*registry.Node, error) {
	checks, _, err := d.client.Health().Checks(service, &api.QueryOptions{Filter: "Status==passing"})
	if err != nil {
		return nil, errors.New(fmt.Sprintf("unable to query health checks, err: %v", err))
	}

	if len(checks) == 0 {
		return nil, consulerrors.AvailableNodeNotExist
	}

	var nodes []*registry.Node
	for _, check := range checks {
		as, _, err := d.client.Agent().Service(check.ServiceID, nil)
		if err != nil {
			return nil, errors.New(fmt.Sprintf("unable to query service, err: %v", err))
		}
		var md = map[string]string{"CheckID": check.CheckID}
		node := &registry.Node{Id: as.ID, Address: fmt.Sprintf("%s:%d", as.Address, as.Port), Metadata: md}
		nodes = append(nodes, node)
	}

	if !reflect.DeepEqual(d.nodes, nodes) {
		d.nodes = nodes
		d.next = d.Strategy([]*registry.Service{{Nodes: nodes}})
	}

	selectedNode, err := d.next()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("unable to select node in selector, err: %v", err))
	}

	return selectedNode, nil
}
