// add package in v.1.0.2
// this package is for declaring custom router, overriding *gin.Engine
// custom_router.go is file that declare custom router struct & initializer

package router

import "github.com/gin-gonic/gin"

// customRouter basically embedding *gin.Engine, and declare to override additional function in basic router
// Additional function is run closure after & before server start or end, etc ...
type customRouter struct {
	*gin.Engine
	beforeStart []func()
}

func NewCustom(baseRouter *gin.Engine) (router *customRouter) {
	router = &customRouter{
		Engine: baseRouter,
	}
	router.beforeStart = []func(){}

	return
}
