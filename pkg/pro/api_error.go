package pro

import (
	"fmt"
	"net/http"
)

// APIError represents an error from the Atmos Pro API that includes the HTTP status code.
// This allows retry logic to distinguish retryable errors (401, 5xx) from non-retryable ones (400, 403, 404).
type APIError struct {
	StatusCode int
	Operation  string
	Err        error
}

// Error returns a human-readable description of the API error.
//
// For 4xx responses the server's error message is already user-facing
// ("Drift detection validation failed: ..."), so the redundant "HTTP <code>:"
// prefix is dropped — the user sees "UploadInstances: <message>".
//
// For 5xx and other errors the prefix is kept because the inner cause is
// usually a generic fallback like "API request failed with status N" and
// the explicit status code helps both users and support.
func (e *APIError) Error() string {
	if e.StatusCode >= http.StatusBadRequest && e.StatusCode < http.StatusInternalServerError {
		return fmt.Sprintf("%s: %v", e.Operation, e.Err)
	}
	return fmt.Sprintf("%s: HTTP %d: %v", e.Operation, e.StatusCode, e.Err)
}

// Unwrap returns the underlying error for use with errors.Is and errors.As.
func (e *APIError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error represents a retryable HTTP status (401 or 5xx).
func (e *APIError) IsRetryable() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode >= http.StatusInternalServerError
}

// IsAuthError returns true if the error is a 401 Unauthorized.
func (e *APIError) IsAuthError() bool {
	return e.StatusCode == http.StatusUnauthorized
}
