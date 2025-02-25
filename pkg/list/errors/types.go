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

// NoMetadataFoundError represents an error when no metadata is found with a given query.
type NoMetadataFoundError struct {
	Query string
}

func (e *NoMetadataFoundError) Error() string {
	return fmt.Sprintf("no metadata found in any stacks with query '%s'", e.Query)
}

// MetadataFilteringError represents an error when filtering and listing metadata.
type MetadataFilteringError struct {
	Cause error
}

func (e *MetadataFilteringError) Error() string {
	return fmt.Sprintf("error filtering and listing metadata: %v", e.Cause)
}

func (e *MetadataFilteringError) Unwrap() error {
	return e.Cause
}

// CommonFlagsError represents an error getting common flags.
type CommonFlagsError struct {
	Cause error
}

func (e *CommonFlagsError) Error() string {
	return fmt.Sprintf("error getting common flags: %v", e.Cause)
}

func (e *CommonFlagsError) Unwrap() error {
	return e.Cause
}

// InitConfigError represents an error initializing CLI config.
type InitConfigError struct {
	Cause error
}

func (e *InitConfigError) Error() string {
	return fmt.Sprintf("error initializing CLI config: %v", e.Cause)
}

func (e *InitConfigError) Unwrap() error {
	return e.Cause
}

// DescribeStacksError represents an error describing stacks.
type DescribeStacksError struct {
	Cause error
}

func (e *DescribeStacksError) Error() string {
	return fmt.Sprintf("error describing stacks: %v", e.Cause)
}

func (e *DescribeStacksError) Unwrap() error {
	return e.Cause
}

// NoSettingsFoundError represents an error when no settings are found with a given query.
type NoSettingsFoundError struct {
	Query string
}

func (e *NoSettingsFoundError) Error() string {
	return fmt.Sprintf("no settings found in any stacks with query '%s'", e.Query)
}

// SettingsFilteringError represents an error when filtering and listing settings.
type SettingsFilteringError struct {
	Cause error
}

func (e *SettingsFilteringError) Error() string {
	return fmt.Sprintf("error filtering and listing settings: %v", e.Cause)
}

func (e *SettingsFilteringError) Unwrap() error {
	return e.Cause
}
