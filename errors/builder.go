package errors

import (
	"fmt"

	"github.com/cockroachdb/errors"
)

// ErrorBuilder provides a fluent API for constructing enriched errors.
type ErrorBuilder struct {
	err      error
	hints    []string
	exitCode *int
}

// Build creates a new ErrorBuilder from a base error.
func Build(err error) *ErrorBuilder {
	return &ErrorBuilder{err: err}
}

// WithHint adds a user-facing hint to the error.
// Multiple hints can be added and will be displayed to users.
func (b *ErrorBuilder) WithHint(hint string) *ErrorBuilder {
	b.hints = append(b.hints, hint)
	return b
}

// WithHintf adds a formatted user-facing hint to the error.
func (b *ErrorBuilder) WithHintf(format string, args ...interface{}) *ErrorBuilder {
	b.hints = append(b.hints, fmt.Sprintf(format, args...))
	return b
}

// WithContext adds safe structured context to the error.
// This is PII-safe and will be included in error reporting.
func (b *ErrorBuilder) WithContext(key string, value interface{}) *ErrorBuilder {
	b.err = errors.WithSafeDetails(b.err, key, value)
	return b
}

// WithExitCode attaches an exit code to the error.
func (b *ErrorBuilder) WithExitCode(code int) *ErrorBuilder {
	b.exitCode = &code
	return b
}

// Err finalizes and returns the enriched error.
func (b *ErrorBuilder) Err() error {
	if b.err == nil {
		return nil
	}

	err := b.err

	// Add all hints.
	for _, hint := range b.hints {
		err = errors.WithHint(err, hint)
	}

	// Add exit code if specified.
	if b.exitCode != nil {
		err = WithExitCode(err, *b.exitCode)
	}

	return err
}
