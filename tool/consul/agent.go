package consul

import "github.com/micro/go-micro/v2/registry"

type Agent interface {
	GetNextServiceNode(serviceID string) (*registry.Node, error)
	SetFailTTLHealth(checkID, note string) error
	SetPassTTLHealth(checkID, note string) error
}
