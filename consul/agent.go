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
	// add in v.1.0.2
	ChangeAllServiceNodes() error

	// method to refresh specific service node list
	// add in v.1.0.2
	ChangeServiceNodes(ServiceName) error

	// get specific service node based on memory saved in change method
	GetNextServiceNode(ServiceName) (*registry.Node, error)

	// change ttl health of specific check to fail
	FailTTLHealth(checkID, note string) error

	// change ttl health of specific check to pass
	PassTTLHealth(checkID, note string) error

	// return closure that register service node
	// add in v.1.0.2 (move from tool/closure/consul.go)
	ServiceNodeRegistry(server.Server) func() error

	// return closure that deregister service node
	// add in v.1.0.2 (move from tool/closure/consul.go)
	ServiceNodeDeregistry(server.Server) func() error

	// get redis connection config from consul KV
	// add in v.1.0.3
	GetRedisConfigFromKV(key string) (RedisConfigKV, error)
}
