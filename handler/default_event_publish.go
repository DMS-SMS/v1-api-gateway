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

	// 이벤트 발생
	// 모든 서비스 조회 새로고침 (해당 서비스 포함)
	// 해당 서비스에 대한 연결을 새로 맺어야 한다는 뜻이니까 health checker도 받아서 ping 보냄
	// 참고로 해당 서비스가 새로 시작될 때도 이벤트 발생 필요. (없었을 수도 있으니)

	if c.GetHeader("Type") == "checks" {
		c.Status(http.StatusOK)
		return
	}
}
