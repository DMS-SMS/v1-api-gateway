package jwt

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
)

func ParseUUIDClaimsFrom(tokenStr string) (claims *UUIDClaims, err error) {
	token, err := jwt.ParseWithClaims(tokenStr, &UUIDClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(jwtKey), nil
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
