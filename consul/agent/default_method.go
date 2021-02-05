// Add file in v.1.0.2
// default_method.go is file to declare method of default struct

package agent

import (
	"encoding/json"
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

// move from tool/agent/default.go to agent/default_method.go
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

// move from tool/agent/default.go to agent/default_method.go in v.1.0.2
func (d *_default) FailTTLHealth(checkID, note string) (err error) {
	return d.client.Agent().FailTTL(checkID, note)
}

// move from tool/agent/default.go to agent/default_method.go in v.1.0.2
func (d *_default) PassTTLHealth(checkID, note string) (err error) {
	return d.client.Agent().PassTTL(checkID, note)
}

func (d *_default) GetRedisConfigFromKV(key string) (conf consul.RedisConfigKV, err error) {
	kv, _, err := d.client.KV().Get(key, nil)
	if err != nil {
		err = errors.New(fmt.Sprintf("unable to get %s KV from consul, err: %v", key, err.Error()))
		return
	}

	if err = json.Unmarshal(kv.Value, &conf); err != nil {
		err = errors.New(fmt.Sprintf("error occurs while unmarshal KV value into struct, err: %v", err.Error()))
		return
	}

	if err = d.validator.Struct(&conf); err != nil {
		err = errors.New(fmt.Sprintf("invalid %s KV value, err: %v", key, err.Error()))
		return
	}

	return
}
