// add package in v.1.0.2
// move directory from tool/consul to consul in v.1.0.2

package consul

import (
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/server"
)

type ServiceName string

type Agent interface {
	// method to refresh all service node list
	ChangeAllServiceNodes() error         // add in v.1.0.2
	// method to refresh specific service node list
	ChangeServiceNodes(ServiceName) error // add in v.1.0.2
	GetNextServiceNode(ServiceName) (*registry.Node, error)
	ServiceNodeRegistry(server.Server) func() error   // add in v.1.0.2 (move from tool/closure/consul.go)
	ServiceNodeDeregistry(server.Server) func() error // add in v.1.0.2 (move from tool/closure/consul.go)
}
