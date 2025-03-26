package errors

import (
	"fmt"
)

// AtmosError represents a structured error with rich context.
type AtmosError struct {
	Base     error                  // Underlying error for unwrapping
	Message  string                 // Human-readable message
	Meta     map[string]interface{} // Contextual metadata
	Tips     []string               // User guidance
	ExitCode int                    // Process exit code
}

// Error implements the error interface.
func (e *AtmosError) Error() string {
	return e.Message
}

// Unwrap returns the underlying error.
func (e *AtmosError) Unwrap() error {
	return e.Base
}

// Fields returns the metadata fields for logging.
func (e *AtmosError) Fields() map[string]interface{} {
	fields := make(map[string]interface{})
	for k, v := range e.Meta {
		fields[k] = v
	}
	return fields
}

// WithContext adds context key-value pairs to the error metadata.
func (e *AtmosError) WithContext(keyvals ...interface{}) *AtmosError {
	if len(keyvals)%2 != 0 {
		// If odd number of arguments, add empty string to make it even
		keyvals = append(keyvals, "")
	}

	if e.Meta == nil {
		e.Meta = make(map[string]interface{})
	}

	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keyvals[i])
		}
		e.Meta[key] = keyvals[i+1]
	}

	return e
}

// WithTips adds user guidance tips to the error.
func (e *AtmosError) WithTips(tips ...string) *AtmosError {
	e.Tips = append(e.Tips, tips...)
	return e
}

// NewAtmosError creates a new structured error with the given message and base error.
func NewAtmosError(message string, baseErr error) *AtmosError {
	return &AtmosError{
		Message: message,
		Base:    baseErr,
		Meta:    make(map[string]interface{}),
	}
}

// NewBaseError creates a new base error that can be used for error checking.
func NewBaseError(message string) *AtmosError {
	return NewAtmosError(message, nil)
}
