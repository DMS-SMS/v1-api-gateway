package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/mervick/aes-everywhere/go/aes256"
	"net/http"
	"regexp"
)

type securityFilter struct {
	basePlain         string
	passPhrase        string
	filteredSecurity  map[string]bool
	onceUsedSecurity  map[string]bool
	basePlainTemplate *regexp.Regexp
}

func (s *securityFilter) filterSecurity(c *gin.Context) {
	respFor407 := struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status: http.StatusProxyAuthRequired,
		Message: "please send the request through the proxy",
	}

	security := c.GetHeader("Request-Filter")
	if security == "" {
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	if s.filteredSecurity[security] {
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	if s.onceUsedSecurity[security] {
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	decrypted := aes256.Decrypt(security, s.passPhrase)
	if decrypted == "" || !s.basePlainTemplate.MatchString(decrypted) {
		s.filteredSecurity[security] = true
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	s.onceUsedSecurity[security] = true
	c.Next()
}
