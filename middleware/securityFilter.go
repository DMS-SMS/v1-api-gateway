package middleware

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mervick/aes-everywhere/go/aes256"
	"log"
	"net/http"
	"os"
	"regexp"
)

type securityFilter struct {
	basePlain         string
	passPhrase        string
	filteredSecurity  map[string]bool
	onceUsedSecurity  map[string]bool
	basePlainTemplate *regexp.Regexp
}

func SecurityFilter() gin.HandlerFunc {
	basePlain := os.Getenv("SECURITY_BASE_PLAIN")
	if basePlain == "" {
		log.Fatal("please set SECURITY_BASE_PLAIN in environment variable")
	}
	passPhrase := os.Getenv("SECURITY_PASS_PHRASE")
	if passPhrase == "" {
		log.Fatal("please set SECURITY_PASS_PHRASE in environment variable")
	}

	return (&securityFilter{
		basePlain:         basePlain,
		passPhrase:        passPhrase,
		filteredSecurity:  map[string]bool{},
		onceUsedSecurity:  map[string]bool{},
		basePlainTemplate: regexp.MustCompile(fmt.Sprintf("^%s:\\d{10}", basePlain)),
	}).filterSecurity
}

func (s *securityFilter) filterSecurity(c *gin.Context) {
	respFor407 := struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status: http.StatusProxyAuthRequired,
		Message: "please send the request through the proxy",
	}

	security := c.GetHeader("Request-Security")
	if security == "" {
		fmt.Println(1)
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	if s.filteredSecurity[security] {
		fmt.Println(2)
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	if s.onceUsedSecurity[security] {
		fmt.Println(3)
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	decrypted := aes256.Decrypt(security, s.passPhrase)
	if decrypted == "" || !s.basePlainTemplate.MatchString(decrypted) {
		fmt.Println(4)
		s.filteredSecurity[security] = true
		c.AbortWithStatusJSON(http.StatusProxyAuthRequired, respFor407)
		return
	}

	s.onceUsedSecurity[security] = true
	c.Next()
}
