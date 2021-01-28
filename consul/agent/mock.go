// add package in v.1.0.2
// clone from tool/consul/agent in club

package agent

import (
	"gateway/consul"
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/server"
	"github.com/stretchr/testify/mock"
)

type _mock struct {
	mock *mock.Mock
}

func Mock(mock *mock.Mock) _mock {
	return _mock{mock: mock}
}

func (m _mock) ChangeAllServiceNodes() error {
	return m.mock.Called().Error(0)
}

func (m _mock) ChangeServiceNodes(service consul.ServiceName) error {
	return m.mock.Called().Error(0)
}

func (m _mock) GetNextServiceNode(service consul.ServiceName) (*registry.Node, error) {
	args := m.mock.Called(service)
	return args.Get(0).(*registry.Node), args.Error(1)
}

func (m _mock) FailTTLHealth(checkID, note string) error {
	return m.mock.Called().Error(0)
}

func (m _mock) PassTTLHealth(checkID, note string) error {
	return m.mock.Called().Error(0)
}

func (m _mock) ServiceNodeRegistry(server server.Server) func() error {
	return m.mock.Called(server).Get(0).(func() error)
}

func (m _mock) ServiceNodeDeregistry(server server.Server) func() error {
	return m.mock.Called(server).Get(0).(func() error)
}

func (m _mock) GetRedisConfigFromKV(key string) (consul.RedisConfigKV, error) {
	args := m.mock.Called(key)
	return args.Get(0).(consul.RedisConfigKV), args.Error(1)
}
