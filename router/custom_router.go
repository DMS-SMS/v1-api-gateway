// add package in v.1.0.2
// custom_router.go is file that declare method, overriding method of customRouter

package router

import (
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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

// method that return custom router group having method declared in custom_group.go
func (r *customRouter) CustomGroup(relativePath string, handlers ...gin.HandlerFunc) *customRouterGroup {
	return &customRouterGroup{
		RouterGroup: r.RouterGroup.Group(relativePath, handlers...),
		Validator:   validator.New(),
	}
}
