// add file in v.1.0.5
// default_xlsx_handle.go is file that declare xlsx handling method

package handler

import (
	"regexp"
)

var (
	studentNumberRegex = regexp.MustCompile("(^[1-3][1-4])([0-1][0-9]$|20|21)")
	nameRegex = regexp.MustCompile("^[가-힣]+$")
	phoneNumberRegex = regexp.MustCompile("^\"\\d{2,3}[-_.]?\\d{3,4}[-_.]?\\d{4}\"$")
	blankRegex = regexp.MustCompile("(^[ ]+$)|(^$)")
)
