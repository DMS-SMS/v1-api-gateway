// add file in v.1.0.3
// instance_registry.go is file that save all instance of each request entity in map
// this registry is used to declare new instance with string in middleware.RequestValidator

package registry

import (
	"log"
	"regexp"
)

var globalInstance = &requestInstance{}

type requestInstance map[string]interface{}

// register new instance key as string (only regex "^.Request$" accept)
func (ri *requestInstance) registerInstance(instance string) {
	if !regexp.MustCompile("^.*Request$").MatchString(instance) {
		log.Fatalf("regex of all struct in request entity files must be \"^.*Request$\", struct name: %s", instance)
	}

	if sample, ok := requestSamples[instance]; !ok {
		log.Fatalf("register struct must be in request sample, struct name: %s\n", instance)
	} else {
		(*ri)[instance] = sample
	}
}
