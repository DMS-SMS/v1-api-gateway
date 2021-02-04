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

// finally call origin POST, GET, DELETE, PATCH, PUT method of RouterGroup
func (g *customRouterGroup) post(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.RouterGroup.POST(relativePath, append([]gin.HandlerFunc{handler}, handlers...)...)
}

func (g *customRouterGroup) get(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.RouterGroup.GET(relativePath, append([]gin.HandlerFunc{handler}, handlers...)...)
}

func (g *customRouterGroup) delete(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.RouterGroup.DELETE(relativePath, append([]gin.HandlerFunc{handler}, handlers...)...)
}

func (g *customRouterGroup) patch(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.RouterGroup.PATCH(relativePath, append([]gin.HandlerFunc{handler}, handlers...)...)
}

func (g *customRouterGroup) put(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	return g.RouterGroup.PUT(relativePath, append([]gin.HandlerFunc{handler}, handlers...)...)
}
