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
		Validator:   g.Validator,
	}
}

// add authenticator & request validator middleware in front of handlers before routing
func (g *customRouterGroup) POSTWithAuth(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	prefixHandlers := []gin.HandlerFunc{middleware.Authenticator(), middleware.RequestValidator(g.Validator, handler)}
	return g.post(relativePath, handler, append(prefixHandlers, handler)...)
}

func (g *customRouterGroup) GETWithAuth(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	prefixHandlers := []gin.HandlerFunc{middleware.Authenticator(), middleware.RequestValidator(g.Validator, handler)}
	return g.get(relativePath, handler, append(prefixHandlers, handler)...)
}

func (g *customRouterGroup) DELETEWithAuth(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	prefixHandlers := []gin.HandlerFunc{middleware.Authenticator(), middleware.RequestValidator(g.Validator, handler)}
	return g.delete(relativePath, handler, append(prefixHandlers, handler)...)
}

func (g *customRouterGroup) PATCHWithAuth(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	prefixHandlers := []gin.HandlerFunc{middleware.Authenticator(), middleware.RequestValidator(g.Validator, handler)}
	return g.patch(relativePath, handler, append(prefixHandlers, handler)...)
}

func (g *customRouterGroup) PUTWithAuth(relativePath string, handler gin.HandlerFunc, handlers ...gin.HandlerFunc) gin.IRoutes {
	prefixHandlers := []gin.HandlerFunc{middleware.Authenticator(), middleware.RequestValidator(g.Validator, handler)}
	return g.put(relativePath, handler, append(prefixHandlers, handler)...)
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
