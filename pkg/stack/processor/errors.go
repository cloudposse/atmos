package processor

import "errors"

// Static errors for processor operations.
var (
	// ErrProcessingFailed is returned when processing fails.
	ErrProcessingFailed = errors.New("processing failed")

	// ErrFunctionNotFound is returned when a function is not found.
	ErrFunctionNotFound = errors.New("function not found")

	// ErrInvalidData is returned when the data structure is invalid.
	ErrInvalidData = errors.New("invalid data structure")

	// ErrSkippedFunction is returned when a function is skipped.
	ErrSkippedFunction = errors.New("function skipped")

	// ErrCycleDetected is returned when a dependency cycle is detected.
	ErrCycleDetected = errors.New("dependency cycle detected")

	// ErrNilProcessor is returned when a nil processor is used.
	ErrNilProcessor = errors.New("nil processor")

	// ErrNilContext is returned when a nil context is provided.
	ErrNilContext = errors.New("nil context")
)
