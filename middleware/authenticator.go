// create file in v.1.0.3
// authenticator.go is file that declare authentication handler middleware, using in auth routing

package middleware

import (
	"fmt"
	jwtutil "gateway/tool/jwt"
	respcode "gateway/utils/code/golang"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

func Authenticator() gin.HandlerFunc {
	return func(c *gin.Context) {
		var claims jwtutil.UUIDClaims
		respFor401 := gin.H{
			"status":  http.StatusUnauthorized,
			"code":    0,
			"message": "",
		}

		if c.GetHeader("Authorization") == "" {
			respFor401["code"] = respcode.NoAuthorizationInHeader
			respFor401["message"] = "authorization doesn't exist in header"
			c.AbortWithStatusJSON(http.StatusUnauthorized, respFor401)
			return
		}

		if len(strings.Split(c.GetHeader("Authorization"), " ")) != 2 {
			respFor401["code"] = respcode.InvalidFormatOfAuthorization
			respFor401["message"] = "invalid data format of Authorization"
			c.AbortWithStatusJSON(http.StatusUnauthorized, respFor401)
			return
		}

		authType := strings.Split(c.GetHeader("Authorization"), " ")[0]
		authValue := strings.Split(c.GetHeader("Authorization"), " ")[1]

		switch authType {
		case "Bearer":
			parsedClaims, err := jwtutil.ParseUUIDClaimsFrom(authValue)
			switch assertedErr := err.(type) {
			case nil:
				claims = *parsedClaims
			case *jwt.ValidationError:
				switch assertedErr.Errors {
				case jwt.ValidationErrorSignatureInvalid:
					respFor401["code"] = respcode.InvalidSignatureOfJWT
					respFor401["message"] = "invalid signature of JWT"
				case jwt.ValidationErrorExpired:
					respFor401["code"] = respcode.ExpiredJWTToken
					respFor401["message"] = "expired jwt token"
				case jwt.ValidationErrorClaimsInvalid:
					respFor401["code"] = respcode.InvalidClaimsOfJWT
					respFor401["message"] = "invalid claims of jwt"
				default:
					respFor401["message"] = fmt.Sprintf("unexpected error occurs while parsing JWT, err: %v", err)
				}
				c.AbortWithStatusJSON(http.StatusUnauthorized, respFor401)
				return
			default:
				respFor401["message"] = fmt.Sprintf("error of unexpected type occurs while parsing JWT, err: %v", err)
				c.AbortWithStatusJSON(http.StatusUnauthorized, respFor401)
				return
			}
		default:
			respFor401["code"] = respcode.UnsupportedAuthorization
			respFor401["message"] = fmt.Sprintf("%s is an unacceptable authentication method", authType)
			c.AbortWithStatusJSON(http.StatusUnauthorized, respFor401)
			return
		}

		c.Set("Claims", claims)
		c.Next()
	}
}