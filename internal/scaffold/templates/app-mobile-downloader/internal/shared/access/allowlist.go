package access

import "strings"

func IsAllowedAppEmail(email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	_, ok := allowedAppEmails[email]
	return ok
}

func IsAllowedEditorEmail(email string) bool {
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	_, ok := allowedEditorEmails[email]
	return ok
}

func IsAllowedAnyEmail(email string) bool {
	return IsAllowedAppEmail(email) || IsAllowedEditorEmail(email)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

var allowedAppEmails = map[string]struct{}{}

var allowedEditorEmails = map[string]struct{}{}
