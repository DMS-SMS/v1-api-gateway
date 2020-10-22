package handler

import (
	"fmt"
	jwtutil "gateway/tool/jwt"
	respcode "gateway/utils/code/golang"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"strings"
)

func (_ *_default) checkIfAuthenticated(c *gin.Context) (ok bool, claims jwtutil.UUIDClaims, code int, msg string) {
	if c.GetHeader("Authorization") == "" {
		ok = false
		code = respcode.NoAuthorizationInHeader
		msg = "authorization doesn't exist in header"
		return
	}

	if len(strings.Split(c.GetHeader("Authorization"), " ")) != 2 {
		ok = false
		code = respcode.InvalidAuthorizationFormat
		msg = "invalid data format of Authorization"
		return
	}

	authType := strings.Split(c.GetHeader("Authorization"), " ")[0]
	authValue := strings.Split(c.GetHeader("Authorization"), " ")[1]

	switch authType {
	case "Bearer":
		parsedClaims, err := jwtutil.ParseUUIDClaimsFrom(authValue)
		switch assertedErr := err.(type) {
		case nil:
			ok = true
			claims = *parsedClaims
		case *jwt.ValidationError:
			ok = false
			switch assertedErr.Errors {
			case jwt.ValidationErrorSignatureInvalid:
				code = respcode.InvalidSignatureOfJWT
				msg = "invalid signature of JWT"
			case jwt.ValidationErrorExpired:
				code = respcode.ExpiredJWTToken
				msg = "expired jwt token"
			case jwt.ValidationErrorClaimsInvalid:
				code = respcode.InvalidClaimsOfJWT
				msg = "invalid claims of jwt"
			default:
				msg = fmt.Sprintf("unexpected error occurs while parsing JWT, err: %v", err)
			}
		default:
			ok = false
			msg = fmt.Sprintf("error of unexpected type occurs while parsing JWT, err: %v", err)
		}
		return
	default:
		ok = false
		code = respcode.InvalidAuthorizationType
		msg = fmt.Sprintf("%s is an unacceptable authentication method", authType)
		return
	}
}
