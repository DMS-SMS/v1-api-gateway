// add file in v.1.0.3
// request_validator.go is file that declare closure bind request body to golang struct & validate request

package middleware

import (
	"github.com/go-playground/validator/v10"
)

// create all closure from this global struct method
var globalValidator *requestValidator

type requestValidator struct {
	validator *validator.Validate
}
