// Add file in v.1.0.2
// default_method.go is file to declare method of default struct

package agent

import (
	"errors"
	"fmt"
	"gateway/consul"
	"github.com/hashicorp/consul/api"
	"github.com/micro/go-micro/v2/registry"
	"reflect"
)

const StatusMustBePassing = "Status==passing"

// call changeServiceNode multiple as count as length of services array with mutex.Lock & Unlock
func (d *_default) ChangeAllServiceNodes() (err error) {
	d.nodeMutex.Lock()
	defer d.nodeMutex.Unlock()

	for _, service := range d.services {
		// when tmpErr is nil
		if tmpErr := d.changeServiceNodes(service); tmpErr == nil {
			continue
		// when tmpErr is nil, but err is not nil
		} else if err == nil {
			err = tmpErr
		// when tmpErr and err both not nil
		} else {
			err = errors.New(err.Error() + " " + tmpErr.Error())
		}
	}
	return
}

// call changeServiceNode once with mutex.Lock & Unlock
func (d *_default) ChangeServiceNodes(service consul.ServiceName) (err error) {
	d.nodeMutex.Lock()
	defer d.nodeMutex.Unlock()

	err = d.changeServiceNodes(service)
	return
}

// private method to handle business logic of changing specific service node list
func (d *_default) changeServiceNodes(service consul.ServiceName) error {
	checks, _, err := d.client.Health().Checks(string(service), &api.QueryOptions{Filter: StatusMustBePassing})
	if err != nil {
		return errors.New(fmt.Sprintf("unable to query health checkes, err: %v", err))
	}

	var nodes []*registry.Node
	for _, check := range checks {
		as, _, err := d.client.Agent().Service(check.ServiceID, nil)
		if err != nil {
			return errors.New(fmt.Sprintf("unable to query service, err: %v", err))
		}
		var md = map[string]string{"CheckID": check.CheckID}
		node := &registry.Node{Id: as.ID, Address: fmt.Sprintf("%s:%d", as.Address, as.Port), Metadata: md}
		nodes = append(nodes, node)
	}

	if !reflect.DeepEqual(d.nodes[service], nodes) {
		d.nodes[service] = nodes
		d.next[service] = d.Strategy([]*registry.Service{{Nodes: nodes}})
	}

	return nil
}

// move from agent/default.go to agent/default_method.go
// migrate change logic to changeServiceNodes method in v.1.0.2
func (d *_default) GetNextServiceNode(service consul.ServiceName) (*registry.Node, error) {
	d.nodeMutex.RLock()
	defer d.nodeMutex.RUnlock()
	
	if !d.checkIfExistService(service) {
		return nil, ErrUndefinedService
	}
	
	if _, exist := d.nodes[service]; !exist {
		_ = d.changeServiceNodes(service)
		return nil, ErrUnavailableService
	}

	if len(d.nodes[service]) == 0 {
		return nil, ErrAvailableNodeNotFound
	}

	selectedNode, err := d.next[service]()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("unable to select node in selector, err: %v", err))
	}

	return selectedNode, nil
}

// check if _default.services array contain srv parameter
func (d *_default) checkIfExistService(srv consul.ServiceName) (exist bool) {
	for _, service := range d.services {
		if service == srv {
			exist = true
			return
		}
	}

	exist = false
	return
}
