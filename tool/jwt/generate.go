package jwt

import (
	"github.com/dgrijalva/jwt-go"
	"log"
	"os"
)

var jwtKey string
func init() {
	if jwtKey = os.Getenv("JWT_SECRET_KEY"); jwtKey == "" {
		log.Fatal("please set JWT_SECRET_KEY in environment e")
	}
}

func GenerateStringWithClaims(claims jwt.Claims, method jwt.SigningMethod) (ss string, err error) {
	ss, err = jwt.NewWithClaims(method, claims).SignedString([]byte(jwtKey))
	return
}
