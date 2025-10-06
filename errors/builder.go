package errors

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrorBuilder provides a fluent API for constructing enriched errors.
type ErrorBuilder struct {
	err      error
	hints    []string
	context  map[string]interface{}
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

// WithExplanation adds a detailed explanation to the error.
// The explanation provides context about what went wrong and why.
// It will be displayed in a dedicated "## Explanation" section when formatted.
func (b *ErrorBuilder) WithExplanation(explanation string) *ErrorBuilder {
	b.err = errors.WithDetail(b.err, explanation)
	return b
}

// WithExplanationf adds a formatted explanation to the error.
func (b *ErrorBuilder) WithExplanationf(format string, args ...interface{}) *ErrorBuilder {
	return b.WithExplanation(fmt.Sprintf(format, args...))
}

// WithExampleFile adds a code/config example from an embedded markdown file.
// Examples are stored as special hints prefixed with "EXAMPLE:" and displayed
// in a dedicated "## Example" section when formatted.
func (b *ErrorBuilder) WithExampleFile(content string) *ErrorBuilder {
	b.hints = append(b.hints, "EXAMPLE:"+content)
	return b
}

// WithExample adds an inline code/config example.
// For simple cases where creating a separate markdown file is overkill.
func (b *ErrorBuilder) WithExample(example string) *ErrorBuilder {
	return b.WithExampleFile(example)
}

// WithContext adds safe structured context to the error.
// This is PII-safe and will be included in error reporting.
// Context is displayed in verbose mode and sent to Sentry.
func (b *ErrorBuilder) WithContext(key string, value interface{}) *ErrorBuilder {
	if b.context == nil {
		b.context = make(map[string]interface{})
	}
	b.context[key] = value
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

	// Add context if present.
	if len(b.context) > 0 {
		// Sort keys for consistent output.
		keys := make([]string, 0, len(b.context))
		for k := range b.context {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Build format string: "component=%s stack=%s".
		var formatParts []string
		var safeValues []interface{}

		for _, key := range keys {
			formatParts = append(formatParts, key+"=%s")
			safeValues = append(safeValues, errors.Safe(b.context[key]))
		}

		err = errors.WithSafeDetails(err, strings.Join(formatParts, " "), safeValues...)
	}

	// Add exit code if specified.
	if b.exitCode != nil {
		err = WithExitCode(err, *b.exitCode)
	}

	return err
}
