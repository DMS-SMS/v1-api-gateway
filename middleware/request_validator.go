// add file in v.1.0.3
// request_validator.go is file that declare closure bind request body to golang struct & validate request

package middleware

import (
	"fmt"
	"gateway/entity"
	entityregistry "gateway/entity/registry"
	code "gateway/utils/code/golang"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"net/http"
	"reflect"
	"runtime"
	"strings"
)

// create all closure from this global struct method
var globalValidator *requestValidator

type requestValidator struct {
	validator *validator.Validate
}

func RequestValidator(v *validator.Validate, h gin.HandlerFunc) gin.HandlerFunc {
	if globalValidator == nil || globalValidator.validator != v {
		globalValidator = &requestValidator{v}
	}

	return globalValidator.RequestValidator(h)
}

func (r *requestValidator) RequestValidator(h gin.HandlerFunc) gin.HandlerFunc {
	// EX) gateway/handler.(*_default).CreateNewStudent-fm
	fNames := strings.Split(runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name(), ".")
	fName := strings.TrimSuffix(fNames[2], "-fm")

	return func(c *gin.Context) {
		req, ok := entityregistry.GetInstance(fName + "Request")
		if !ok {
			c.Next()
			return
		}
		respFor400 := gin.H{
			"status":  http.StatusBadRequest,
			"code":    0,
			"message": "",
		}

		switch req.(type) {
		case entity.GetScheduleRequest, entity.GetTimeTableRequest:
			if err := c.ShouldBindUri(req); err != nil {
				respFor400["code"] = code.FailToBindRequestToStruct
				respFor400["message"] = fmt.Sprintf("failed to bind uri in request into golang struct, err: %v", err)
				c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
				return
			}
		case entity.GetClubsSortByUpdateTimeRequest, entity.GetRecruitmentsSortByCreateTimeRequest, entity.GetStudentOutingsRequest,
			entity.GetOutingWithFilterRequest, entity.GetAnnouncementsRequest, entity.GetPlaceWithNaverOpenAPIRequest,
			entity.GetStudentUUIDsWithInformRequest, entity.GetTeacherUUIDsWithInformRequest, entity.GetParentUUIDsWithInformRequest:
			if err := c.ShouldBindQuery(req); err != nil {
				respFor400["code"] = code.FailToBindRequestToStruct
				respFor400["message"] = fmt.Sprintf("failed to bind query parameter in request into golang struct, err: %v", err)
				c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
				return
			}
		default:
			switch c.ContentType() {
			case "multipart/form-data":
				if err := c.ShouldBindWith(req, binding.FormMultipart); err != nil {
					respFor400["code"] = code.FailToBindRequestToStruct
					respFor400["message"] = fmt.Sprintf("failed to bind multipart request into golang struct, err: %v", err)
					c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
					return
				}
			case "application/json":
				if err := c.ShouldBindJSON(req); err != nil {
					respFor400["code"] = code.FailToBindRequestToStruct
					respFor400["message"] = fmt.Sprintf("failed to bind json request into golang struct, err: %v", err)
					c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
					return
				}
			case "":
				if err := c.ShouldBindWith(req, binding.Form); err != nil {
					respFor400["code"] = code.FailToBindRequestToStruct
					respFor400["message"] = fmt.Sprintf("failed to bind request into golang struct, err: %v", err)
					c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
					return
				}
				break
			default:
				respFor400["code"] = code.UnsupportedContentType
				respFor400["message"] = fmt.Sprintf("%s is an unsupported content type", c.ContentType())
				c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
				return
			}
		}

		if err := r.validator.Struct(req); err != nil {
			respFor400["code"] = code.IntegrityInvalidRequest
			respFor400["message"] = fmt.Sprintf("request is not valid for integrity constraints, err: %v", err)
			c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
			return
		}

		switch req.(type) {
		case *entity.GetStudentUUIDsWithInformRequest, *entity.GetTeacherUUIDsWithInformRequest, *entity.GetParentUUIDsWithInformRequest:
			emptyValue := reflect.New(reflect.TypeOf(req).Elem()).Elem().Interface()
			if reflect.DeepEqual(reflect.ValueOf(req).Elem().Interface(), emptyValue) {
				respFor400["code"] = code.IntegrityInvalidRequest
				respFor400["message"] = "you must set up at least one parameter"
				c.AbortWithStatusJSON(http.StatusBadRequest, respFor400)
			}
		}

		c.Set("Request", req)
		c.Next()
	}
}
