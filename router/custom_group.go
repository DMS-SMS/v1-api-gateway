// add file in v.1.0.3
// custom_group.go is file that declare method, overriding method of customRouterGroup

package router

import (
	"github.com/gin-gonic/gin"
)

// method that return custom router group having method declared in this file
func (g *customRouterGroup) CustomGroup(relativePath string, handlers ...gin.HandlerFunc) *customRouterGroup {
	return &customRouterGroup{
		RouterGroup: g.RouterGroup.Group(relativePath, handlers...),
	}
}
