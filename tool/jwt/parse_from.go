package jwt

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"io/ioutil"
	"path/filepath"
)

func ParseUUIDClaimsFrom(tokenStr string) (claims *UUIDClaims, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &UUIDClaims{}, func(t *jwt.Token) (jwtKey interface{}, err error) {
		keyPath, err := filepath.Abs("tool/jwt/jwt_key.priv")
		if err != nil {
			return
		}
		jwtKey, err = ioutil.ReadFile(keyPath)
		if err != nil {
			return
		}
		return
	})
	if err != nil {
		return
	}

	claims, ok := token.Claims.(*UUIDClaims)
	if !ok || !token.Valid {
		err = errors.New("that token is invalid for UUIDClaims")
		return
	}
	return
}
