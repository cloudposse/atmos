package output

import (
	sharedoutput "github.com/cloudposse/atmos/pkg/output"
)

// FormatOutputs converts terraform outputs map to the specified format.
//
// Deprecated: this package's formatting logic has moved to the generic
// pkg/output (no Terraform-specific coupling — the CloudFormation component
// type is a second consumer). This wrapper is kept so existing callers of
// pkg/terraform/output are unaffected; new code should call pkg/output directly.
func FormatOutputs(outputs map[string]any, format Format) (string, error) {
	return sharedoutput.FormatOutputs(outputs, format)
}

// FormatOutputsWithOptions converts terraform outputs map to the specified format with options.
func FormatOutputsWithOptions(outputs map[string]any, format Format, opts FormatOptions) (string, error) {
	return sharedoutput.FormatOutputsWithOptions(outputs, format, opts)
}

// FlattenMap recursively flattens nested maps and arrays into a single-level map with compound keys.
func FlattenMap(m map[string]any, prefix, separator string) map[string]any {
	return sharedoutput.FlattenMap(m, prefix, separator)
}

// FormatSingleValue formats a single terraform output value.
func FormatSingleValue(key string, value any, format Format) (string, error) {
	return sharedoutput.FormatSingleValue(key, value, format)
}

// FormatSingleValueWithOptions formats a single terraform output value with options.
func FormatSingleValueWithOptions(key string, value any, format Format, opts FormatOptions) (string, error) {
	return sharedoutput.FormatSingleValueWithOptions(key, value, format, opts)
}
