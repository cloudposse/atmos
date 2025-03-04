package exec

import (
	"fmt"
	"strings"
)

var (
	ErrURIEmpty                      = fmt.Errorf("URI cannot be empty")
	ErrURIExceedsMaxLength           = fmt.Errorf("URI exceeds maximum length of 2048 characters")
	ErrURICannotContainPathTraversal = fmt.Errorf("URI cannot contain path traversal sequences")
	ErrURICannotContainSpaces        = fmt.Errorf("URI cannot contain spaces")
	ErrUnsupportedURIScheme          = fmt.Errorf("unsupported URI scheme")
	ErrInvalidOCIURIFormat           = fmt.Errorf("invalid OCI URI format")
)

const MaxURISize = 2048

// ValidateURI validates URIs.
func ValidateURI(uri string) error {
	if uri == "" {
		return ErrURIEmpty
	}
	// Maximum length check
	if len(uri) > MaxURISize {
		return ErrURIExceedsMaxLength
	}
	// Validate URI format.
	if strings.Contains(uri, "..") {
		return ErrURICannotContainPathTraversal
	}
	if strings.Contains(uri, " ") {
		return ErrURICannotContainSpaces
	}
	// Validate scheme-specific format.
	if strings.HasPrefix(uri, "oci://") {
		if !strings.Contains(uri[6:], "/") {
			return ErrInvalidOCIURIFormat
		}
	} else if strings.Contains(uri, "://") {
		scheme := strings.Split(uri, "://")[0]
		if !IsValidScheme(scheme) {
			return ErrUnsupportedURIScheme
		}
	}
	return nil
}

// IsValidScheme checks if the URL scheme is valid.
func IsValidScheme(scheme string) bool {
	validSchemes := map[string]bool{
		"http":       true,
		"https":      true,
		"git":        true,
		"ssh":        true,
		"git::https": true,
		"git::ssh":   true,
	}
	return validSchemes[scheme]
}
