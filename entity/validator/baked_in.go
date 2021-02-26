package validator

import (
	"github.com/go-playground/validator/v10"
	"log"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func isValidateUUID(fl validator.FieldLevel) bool {
	if fl.Field().String() == "" {
		return true
	}

	switch fl.Param() {
	case "admin":
		return adminUUIDRegex.MatchString(fl.Field().String())
	case "student":
		return studentUUIDRegex.MatchString(fl.Field().String())
	case "teacher":
		return teacherUUIDRegex.MatchString(fl.Field().String())
	case "parent":
		return parentUUIDRegex.MatchString(fl.Field().String())
	case "club":
		return clubUUIDRegex.MatchString(fl.Field().String())
	case "outing":
		return outingUUIDRegex.MatchString(fl.Field().String())
	case "announcement":
		return announcementUUIDRegex.MatchString(fl.Field().String())
	case "recruitment":
		return recruitmentUUIDRegex.MatchString(fl.Field().String())
	}
	return false
}

func isWithinIntRange(fl validator.FieldLevel) bool {
	paramRange := strings.Split(fl.Param(), "~")
	if len(paramRange) != 2 {
		log.Println("please set param like (int)~(int)")
		return false
	}

	start, err := strconv.Atoi(paramRange[0])
	if err != nil {
		log.Printf("please set param like (int)~(int), err: %v\n", err)
		return false
	}
	end, err := strconv.Atoi(paramRange[1])
	if err != nil {
		log.Printf("please set param like (int)~(int), err: %v\n", err)
		return false
	}

	field := int(fl.Field().Int())
	return field >= start && field <= end
}

func isKoreanString(fl validator.FieldLevel) bool {
	b := []byte(fl.Field().String())
	var idx int

	for {
		r, size := utf8.DecodeRune(b[idx:])
		if size == 0 { break }
		if !unicode.Is(unicode.Hangul, r) { return false }
		idx += size
	}
	return true
}

func isPhoneNumber(fl validator.FieldLevel) bool {
	if fl.Field().String() == "" {
		return true
	}
	return phoneNumberRegex.MatchString(fl.Field().String())
}

func isTime(fl validator.FieldLevel) bool {
	if fl.Field().String() == "" {
		return true
	}
	return timeRegex.MatchString(fl.Field().String())
}

func isValidValue(fl validator.FieldLevel) bool {
	availableValues := strings.Split(fl.Param(), "&")
	value := fl.Field().String()
	for _, availableValue := range availableValues {
		if availableValue == value {
			return true
		}
	}
	return false
}

func isCorrectIntLen(fl validator.FieldLevel) bool {
	intLen, _ := strconv.Atoi(fl.Param())
	if len(strconv.Itoa(int(fl.Field().Int()))) == intLen {
		return true
	}
	return false
}
