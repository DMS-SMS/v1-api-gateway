package handler

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *_default) PublishConsulChangeEvent (c *gin.Context) {
	respFor407 := struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status: http.StatusProxyAuthRequired,
		Message: "please send the request through the proxy",
	}

	switch true {
	case c.GetHeader(consulIndexHeader) != "":
	default:
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}
}
