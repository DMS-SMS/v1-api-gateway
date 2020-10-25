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
		code = respcode.InvalidFormatOfAuthorization
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
		code = respcode.UnsupportedAuthorization
		msg = fmt.Sprintf("%s is an unacceptable authentication method", authType)
		return
	}
}

func (h *_default) checkIfValidRequest(c *gin.Context, bindReq interface{}) (ok bool, code int, msg string) {
	switch c.ContentType() {
	case "multipart/form-data":
		break
	case "":
		break
	default:
		ok = false
		code = respcode.UnsupportedContentType
		msg = fmt.Sprintf("%s is an unsupported content type", c.ContentType())
		return
	}

	if err := c.ShouldBind(bindReq); err != nil {
		ok = false
		code = respcode.FailToBindRequestToStruct
		msg = fmt.Sprintf("failed to bind request json into golang struct, err: %v", err)
		return
	}

	if err := h.validate.Struct(bindReq); err != nil {
		ok = false
		code = respcode.IntegrityInvalidRequest
		msg = fmt.Sprintf("request is not valid for integrity constraints, err: %v", err)
		return
	}

	ok = true
	return
}
