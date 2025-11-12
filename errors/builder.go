package errors

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrorBuilder provides a fluent API for constructing enriched errors.
type ErrorBuilder struct {
	err       error
	title     *string
	hints     []string
	context   map[string]interface{}
	exitCode  *int
	sentinels []error // Sentinel errors to mark with errors.Mark()
}

// Build creates a new ErrorBuilder from a base error.
// If the error is a sentinel error (simple errors.New() with no wrapping),
// it will be automatically marked as a sentinel for errors.Is() checks.
func Build(err error) *ErrorBuilder {
	builder := &ErrorBuilder{err: err}

	// If this looks like a sentinel error (simple error with no cause),
	// automatically mark it as a sentinel for errors.Is() checks.
	if err != nil && errors.UnwrapOnce(err) == nil {
		// This is a leaf error (no wrapped cause), likely a sentinel.
		// Check if it's one of our package-level sentinels by comparing the error text.
		// We'll mark it automatically so errors.Is() works.
		builder.sentinels = append(builder.sentinels, err)
	}

	return builder
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

// WithTitle sets a custom error title that will appear as the H1 heading.
// By default, errors use "Error" as the title.
// This allows customizing to "Workflow Error", "Terraform Error", etc.
func (b *ErrorBuilder) WithTitle(title string) *ErrorBuilder {
	b.title = &title
	return b
}

// WithExitCode attaches an exit code to the error.
func (b *ErrorBuilder) WithExitCode(code int) *ErrorBuilder {
	b.exitCode = &code
	return b
}

// WithSentinel marks the error with a sentinel error for errors.Is() checks.
// This uses errors.Mark() to attach the sentinel to the error chain.
// Multiple sentinels can be added and all will be marked.
func (b *ErrorBuilder) WithSentinel(sentinel error) *ErrorBuilder {
	b.sentinels = append(b.sentinels, sentinel)
	return b
}

// Err finalizes and returns the enriched error.
func (b *ErrorBuilder) Err() error {
	if b.err == nil {
		return nil
	}

	err := b.err

	// Add custom title if specified.
	if b.title != nil {
		err = errors.WithHint(err, "TITLE:"+*b.title)
	}

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

	// Mark with sentinel errors for errors.Is() checks.
	// This must be done AFTER all other wrapping to ensure sentinels are at the top level.
	for _, sentinel := range b.sentinels {
		err = errors.Mark(err, sentinel)
	}

	// Add exit code if specified.
	if b.exitCode != nil {
		err = WithExitCode(err, *b.exitCode)
	}

	return err
}
