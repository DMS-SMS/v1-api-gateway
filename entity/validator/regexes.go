package validator

import "regexp"

const (
	adminUUIDRegexString = "^admin-\\d{12}"
	studentUUIDRegexString = "^student-\\d{12}"
	teacherUUIDRegexString = "^teacher-\\d{12}"
	parentUUIDRegexString = "^parent-\\d{12}"
	clubUUIDRegexString = "^club-\\d{12}"
	outingUUIDRegexString = "^outing-\\d{12}"
	announcementUUIDRegexString = "^announcement-\\d{12}"
	recruitmentUUIDRegexString = "^recruitment-\\d{12}"
	timeRegexString = "\\d{4}-\\d{2}-\\d{2}"
	phoneNumberRegexString = "^010\\d{8}"
)

var (
	adminUUIDRegex = regexp.MustCompile(adminUUIDRegexString)
	studentUUIDRegex = regexp.MustCompile(studentUUIDRegexString)
	teacherUUIDRegex = regexp.MustCompile(teacherUUIDRegexString)
	parentUUIDRegex = regexp.MustCompile(parentUUIDRegexString)
	clubUUIDRegex = regexp.MustCompile(clubUUIDRegexString)
	outingUUIDRegex = regexp.MustCompile(outingUUIDRegexString)
	announcementUUIDRegex = regexp.MustCompile(announcementUUIDRegexString)
	recruitmentUUIDRegex = regexp.MustCompile(recruitmentUUIDRegexString)
	timeRegex = regexp.MustCompile(timeRegexString)
	phoneNumberRegex = regexp.MustCompile(phoneNumberRegexString)
)
