// add file in v.1.0.3
// request_validator.go is file that declare closure bind request body to golang struct & validate request

package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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

	}
}
