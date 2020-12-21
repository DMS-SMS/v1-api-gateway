package middleware

type securityFilter struct {
	basePlain      string
	passPhrase     string
	filteredSecure map[string]bool
}
