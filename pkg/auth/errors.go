package auth

import (
	"strings"
	"time"
)

// FormatProfile formats profile information for error display.
// Returns comma-separated profile names or "(not set)" if empty.
func FormatProfile(profiles []string) string {
	if len(profiles) == 0 {
		return "(not set)"
	}
	return strings.Join(profiles, ", ")
}

// FormatExpiration formats expiration time for error display.
// Returns RFC3339 formatted time or empty string if nil.
func FormatExpiration(expiration *time.Time) string {
	if expiration == nil {
		return ""
	}
	return expiration.Format(time.RFC3339)
}
