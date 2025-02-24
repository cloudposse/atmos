package errors

import (
	"fmt"
	"strings"
)

// NoValuesFoundError represents an error when no values are found for a component.
type NoValuesFoundError struct {
	Component string
	Query     string
}

func (e *NoValuesFoundError) Error() string {
	if e.Query != "" {
		return fmt.Sprintf("no values found for component '%s' with query '%s'", e.Component, e.Query)
	}
	return fmt.Sprintf("no values found for component '%s'", e.Component)
}

// InvalidFormatError represents an error when an invalid format is specified.
type InvalidFormatError struct {
	Format string
	Valid  []string
}

func (e *InvalidFormatError) Error() string {
	return fmt.Sprintf("invalid format '%s'. Valid formats are: %s", e.Format, strings.Join(e.Valid, ", "))
}

// QueryError represents an error when processing a query.
type QueryError struct {
	Query string
	Cause error
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("error processing query '%s': %v", e.Query, e.Cause)
}

func (e *QueryError) Unwrap() error {
	return e.Cause
}

// StackPatternError represents an error with stack pattern matching.
type StackPatternError struct {
	Pattern string
	Cause   error
}

func (e *StackPatternError) Error() string {
	return fmt.Sprintf("invalid stack pattern '%s': %v", e.Pattern, e.Cause)
}

func (e *StackPatternError) Unwrap() error {
	return e.Cause
}
