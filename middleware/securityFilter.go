package middleware

import (
	"regexp"
)

type securityFilter struct {
	basePlain         string
	passPhrase        string
	filteredSecurity  map[string]bool
	onceUsedSecurity  map[string]bool
	basePlainTemplate *regexp.Regexp
}
