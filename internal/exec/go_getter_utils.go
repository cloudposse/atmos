package exec

import (
	"fmt"
	"strings"
)

var (
	ErrUnsupportedUriScheme              = fmt.Errorf("unsupported URI scheme")
	ErrUnsupportedOciUriScheme           = fmt.Errorf("unsupported OCI URI scheme")
	ErrSpaceInURI                        = fmt.Errorf("URI cannot contain spaces")
	ErrURIContainsPathTraversalSequences = fmt.Errorf("URI cannot contain path traversal sequences")
	ErrURIMaximumLengthExceeded          = fmt.Errorf("URI exceeds maximum length")
	ErrURIEmpty                          = fmt.Errorf("URI cannot be empty")
)

// ValidateURI validates URIs.
func ValidateURI(uri string) error {
	if uri == "" {
		return ErrURIEmpty
	}
	// Maximum length check.
	if len(uri) > 2048 {
		return ErrURIMaximumLengthExceeded
	}
	// Validate URI format.
	if strings.Contains(uri, "..") {
		return ErrURIContainsPathTraversalSequences
	}
	if strings.Contains(uri, " ") {
		return ErrSpaceInURI
	}
	// Validate scheme-specific format.
	if strings.HasPrefix(uri, "oci://") {
		if !strings.Contains(uri[6:], "/") {
			return ErrUnsupportedOciUriScheme
		}
	} else if strings.Contains(uri, "://") {
		scheme := strings.Split(uri, "://")[0]
		if !IsValidScheme(scheme) {
			return ErrUnsupportedUriScheme
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
