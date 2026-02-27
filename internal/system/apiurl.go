package system

import "strings"

// IsAllowedAPIURL returns true if the URL uses HTTPS or targets localhost.
// Used to validate OPENBOOT_API_URL environment variable.
func IsAllowedAPIURL(u string) bool {
	if strings.HasPrefix(u, "https://") {
		return true
	}
	if strings.HasPrefix(u, "http://localhost") ||
		strings.HasPrefix(u, "http://127.0.0.1") ||
		strings.HasPrefix(u, "http://[::1]") {
		return true
	}
	return false
}
