package handler

import (
	"fmt"
	"gateway/entity"
	jwtutil "gateway/tool/jwt"
	respcode "gateway/utils/code/golang"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
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
	switch bindReq.(type) {
	case *entity.GetScheduleRequest, *entity.GetTimeTableRequest:
		if err := c.ShouldBindUri(bindReq); err != nil {
			ok = false
			code = respcode.FailToBindRequestToStruct
			msg = fmt.Sprintf("failed to bind uri in request into golang struct, err: %v", err)
			return
		}
	case *entity.GetClubsSortByUpdateTimeRequest, *entity.GetRecruitmentsSortByCreateTimeRequest, *entity.GetStudentOutingsRequest,
		 *entity.GetOutingWithFilterRequest, *entity.GetAnnouncementsRequest, *entity.GetPlaceWithNaverOpenAPIRequest,
		 *entity.GetStudentUUIDsWithInformRequest, *entity.GetTeacherUUIDsWithInformRequest, *entity.GetParentUUIDsWithInformRequest:
		if err := c.ShouldBindQuery(bindReq); err != nil {
			ok = false
			code = respcode.FailToBindRequestToStruct
			msg = fmt.Sprintf("failed to bind query parameter in request into golang struct, err: %v", err)
			return
		}
	default:
		switch c.ContentType() {
		case "multipart/form-data":
			if err := c.ShouldBindWith(bindReq, binding.FormMultipart); err != nil {
				ok = false
				code = respcode.FailToBindRequestToStruct
				msg = fmt.Sprintf("failed to bind multipart request into golang struct, err: %v", err)
				return
			}
		case "application/json":
			if err := c.ShouldBindJSON(bindReq); err != nil {
				ok = false
				code = respcode.FailToBindRequestToStruct
				msg = fmt.Sprintf("failed to bind json request into golang struct, err: %v", err)
				return
			}
		case "":
			if err := c.ShouldBindWith(bindReq, binding.Form); err != nil {
				ok = false
				code = respcode.FailToBindRequestToStruct
				msg = fmt.Sprintf("failed to bind request into golang struct, err: %v", err)
				return
			}
			break
		default:
			ok = false
			code = respcode.UnsupportedContentType
			msg = fmt.Sprintf("%s is an unsupported content type", c.ContentType())
			return
		}
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
