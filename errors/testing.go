package errors

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// Testing helpers for validating error context in tests.
//
// These functions help test that errors built with the error builder pattern
// have the correct context metadata attached. They traverse the error chain
// to find context key-value pairs stored via WithContext().
//
// Example test usage:
//
//	func TestMyFunction_ErrorContext(t *testing.T) {
//	    err := myFunction("invalid-input")
//
//	    // Check error type.
//	    assert.ErrorIs(t, err, ErrInvalidInput)
//
//	    // Check context was attached correctly.
//	    assert.True(t, errUtils.HasContext(err, "input", "invalid-input"))
//	    assert.True(t, errUtils.HasContext(err, "operation", "validate"))
//	}
//
// This ensures the error builder pattern is used correctly and context will
// appear when the error is formatted for display or sent to error tracking.

// HasContext checks if an error's context contains the specified key-value pair.
// This is useful for testing that errors built with the error builder pattern
// have the correct context metadata attached.
//
// Example usage:
//
//	err := Build(ErrVersionFormatInvalid).
//	    WithContext("format", "xml").
//	    Err()
//	assert.True(t, HasContext(err, "format", "xml"))
func HasContext(err error, key, value string) bool {
	if err == nil {
		return false
	}

	// Traverse the entire error chain to find safe details.
	allDetails := errors.GetAllSafeDetails(err)

	// Parse each SafeDetails entry into key-value pairs.
	// Format is "key1=value1 key2=value2".
	for _, payload := range allDetails {
		for _, detail := range payload.SafeDetails {
			detailStr := detail
			pairs := strings.Split(detailStr, " ")
			for _, pair := range pairs {
				// Split on first '=' to handle values with '=' in them.
				parts := strings.SplitN(pair, "=", 2)
				if len(parts) == 2 && parts[0] == key && parts[1] == value {
					return true
				}
			}
		}
	}

	return false
}

// GetContext retrieves the value for a given context key from an error.
// Returns the value and true if found, empty string and false otherwise.
//
// Example usage:
//
//	err := Build(ErrVersionFormatInvalid).
//	    WithContext("format", "xml").
//	    WithContext("version", "1.0.0").
//	    Err()
//	format, ok := GetContext(err, "format")
//	assert.True(t, ok)
//	assert.Equal(t, "xml", format)
func GetContext(err error, key string) (string, bool) {
	if err == nil {
		return "", false
	}

	// Traverse the entire error chain to find safe details.
	allDetails := errors.GetAllSafeDetails(err)

	// Look for "key=" prefix in any of the safe details.
	keyPrefix := key + "="
	for _, payload := range allDetails {
		for _, detail := range payload.SafeDetails {
			detailStr := detail
			// Parse "key1=value1 key2=value2" format.
			pairs := strings.Split(detailStr, " ")
			for _, pair := range pairs {
				if strings.HasPrefix(pair, keyPrefix) {
					value := strings.TrimPrefix(pair, keyPrefix)
					return value, true
				}
			}
		}
	}

	return "", false
}
