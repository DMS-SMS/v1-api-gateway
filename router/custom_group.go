// add file in v.1.0.3
// custom_group.go is file that declare method, overriding method of customRouterGroup

package router

import (
	"gateway/middleware"
	"github.com/gin-gonic/gin"
)

// method that return custom router group having method declared in this file
func (g *customRouterGroup) CustomGroup(relativePath string, handlers ...gin.HandlerFunc) *customRouterGroup {
	return &customRouterGroup{
		RouterGroup: g.RouterGroup.Group(relativePath, handlers...),
	}
}

func (g *customRouterGroup) POSTWithAuth(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes {
	handlers = append([]gin.HandlerFunc{middleware.Authenticator()}, handlers...)
	return g.POST(relativePath, handlers...)
}

func (g *customRouterGroup) GETWithAuth(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes {
	handlers = append([]gin.HandlerFunc{middleware.Authenticator()}, handlers...)
	return g.GET(relativePath, handlers...)
}

func (g *customRouterGroup) DELETEWithAuth(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes {
	handlers = append([]gin.HandlerFunc{middleware.Authenticator()}, handlers...)
	return g.DELETE(relativePath, handlers...)
}

func (g *customRouterGroup) PATCHWithAuth(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes {
	handlers = append([]gin.HandlerFunc{middleware.Authenticator()}, handlers...)
	return g.PATCH(relativePath, handlers...)
}

func (g *customRouterGroup) PUTWithAuth(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes {
	handlers = append([]gin.HandlerFunc{middleware.Authenticator()}, handlers...)
	return g.PUT(relativePath, handlers...)
}
