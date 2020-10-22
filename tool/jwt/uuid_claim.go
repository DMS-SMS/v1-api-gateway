package jwt

import "github.com/dgrijalva/jwt-go"

type UUIDClaims struct {
	UUID string `json:"uuid"`
	jwt.StandardClaims
}
