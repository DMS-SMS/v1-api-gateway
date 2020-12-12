package validator

import (
	"github.com/go-playground/validator/v10"
	"log"
)

var entityValidator *validator.Validate

func init() {
	entityValidator = validator.New()

	if err := entityValidator.RegisterValidation("uuid", isValidateUUID); err != nil { log.Fatal(err) } // 문자열 전용
	if err := entityValidator.RegisterValidation("int_range", isWithinIntRange); err != nil { log.Fatal(err) } // 정수 전용
	if err := entityValidator.RegisterValidation("korean", isKoreanString); err != nil { log.Fatal(err) } // 문자열 전용
	if err := entityValidator.RegisterValidation("phone_number", isPhoneNumber); err != nil { log.Fatal(err) } // 문자열 전용
	if err := entityValidator.RegisterValidation("time", isTime); err != nil { log.Fatal(err) } // 문자열 전용
	if err := entityValidator.RegisterValidation("values", isValidValue); err != nil { log.Fatal(err) } // 문자열 전용
	if err := entityValidator.RegisterValidation("int_len", isCorrectIntLen); err != nil { log.Fatal(err) } // 정수 전용
}

func New() *validator.Validate {
	return entityValidator
}
