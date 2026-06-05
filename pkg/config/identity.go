package config

import "strings"

// NormalizeIdentityValue converts boolean false representations to the disabled sentinel value.
// Recognizes: false, False, FALSE, 0, no, No, NO, off, Off, OFF.
// All other values are returned unchanged.
//
// This function is used to normalize identity values from:
// - CLI flags (--identity=false)
// - Environment variables (ATMOS_IDENTITY=false)
// - Configuration files
//
// When a false value is detected, it returns IdentityFlagDisabledValue ("__DISABLED__")
// which signals to the authentication system to skip authentication entirely.
func NormalizeIdentityValue(value string) string {
	if value == "" {
		return ""
	}

	// Check if value represents boolean false.
	switch strings.ToLower(value) {
	case "false", "0", "no", "off":
		return IdentityFlagDisabledValue
	default:
		return value
	}
}
