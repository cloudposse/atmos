package logger

// Common log field keys for consistent structured logging across the codebase.
// Use these constants instead of string literals to ensure consistency and enable refactoring.
const (
	// Infrastructure fields.
	FieldBucket    = "bucket"
	FieldContainer = "container"
	FieldFile      = "file"
	FieldRegion    = "region"

	// Atmos domain fields.
	FieldStack     = "stack"
	FieldComponent = "component"
	FieldFunction  = "function"

	// Operation fields.
	FieldAttempt   = "attempt"
	FieldBackoff   = "backoff"
	FieldAttempts  = "attempts"
	FieldDuration  = "duration"
	FieldErrorCode = "error_code"

	// General fields.
	FieldError = "error"
	FieldKey   = "key"
	FieldValue = "value"
)
