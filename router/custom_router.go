// add package in v.1.0.2
// custom_router.go is file that declare method, overriding method of customRouter

package router

import (
	"log"
	"reflect"
)

// register closure function to execute before run
func (r *customRouter) RegisterBeforeRun(fn ...func() error) {
	r.beforeRun = append(r.beforeRun, fn...)
}

// overriding run method
// add executing function before server run
func (r *customRouter) Run(addr ...string) error {
	for _, fn := range r.beforeRun {
		if err := fn(); err != nil {
			fnName := reflect.TypeOf(fn).Name()
			log.Fatalf("some error occurs while running before run function, func: %s, err: %v\n", fnName, err)
		}
	}

	return r.Engine.Run(addr...)
}
