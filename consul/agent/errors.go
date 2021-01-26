// add package in v.1.0.2
// clone from tool/consul/agent in club

package agent

import "errors"

var (
	ErrAvailableNodeNotFound = errors.New("there is no currently available service node")
	ErrUndefinedService = errors.New("undefined service, please put in agent.Services if you want to use")
	ErrUnavailableService = errors.New("unavailable service, maybe some error occurred when change service nodes")
)
