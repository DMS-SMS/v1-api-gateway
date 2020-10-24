package validator

import "github.com/go-playground/validator/v10"

func isValidateUUID(fl validator.FieldLevel) bool {
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
