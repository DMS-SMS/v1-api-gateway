package consul

import "github.com/micro/go-micro/v2/registry"

type Agent interface {
	GetNextServiceNode(serviceID string) (*registry.Node, error)
	FailTTLHealth(checkID, note string) error
	PassTTLHealth(checkID, note string) error
}
