// add package in v.1.0.2
// this package is for declaring custom router, overriding *gin.Engine
// custom_router.go is file that declare custom router struct & initializer

package router

import "github.com/gin-gonic/gin"

// customRouter basically embedding *gin.Engine, and declare to override additional function in basic router
// Additional function is run closure after & before server start or end, etc ...
type customRouter struct {
	*gin.Engine
	beforeRun []func() error
}

func New(baseRouter *gin.Engine) (router *customRouter) {
	router = &customRouter{
		Engine: baseRouter,
	}
	router.beforeRun = []func() error{}

	return
}

// register closure function to execute before run
func (r *customRouter) RegisterBeforeRun(fn ...func() error) {
	r.beforeRun = append(r.beforeRun, fn...)
}