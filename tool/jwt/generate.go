package jwt

import (
	"github.com/dgrijalva/jwt-go"
	"io/ioutil"
	"path/filepath"
)

func GenerateStringWithClaims(claims jwt.Claims, method jwt.SigningMethod) (ss string, err error) {
	keyPath, err := filepath.Abs("tool/jwt/jwt_key.priv")
	if err != nil {
		return
	}
	jwtKey, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return
	}

	ss, err = jwt.NewWithClaims(method, claims).SignedString(jwtKey)
	return
}
