package jwt

import "github.com/dgrijalva/jwt-go"

type UUIDClaims struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	jwt.StandardClaims
}
