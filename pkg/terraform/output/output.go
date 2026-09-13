// Package output provides functionality to format and export Terraform outputs
// in various formats suitable for CI/CD workflows, scripts, and configuration files.
//
// The formatting logic itself lives in the generic pkg/output (lifted out because
// none of it is Terraform-specific — the aws/cloudformation component type is a
// second consumer). This package re-exports it via type aliases so existing
// callers are unaffected.
package output

import (
	sharedoutput "github.com/cloudposse/atmos/pkg/output"
)

// Format represents output format types.
type Format = sharedoutput.Format

const (
	// FormatJSON outputs as JSON object: {"key": "value"}.
	FormatJSON = sharedoutput.FormatJSON
	// FormatYAML outputs as YAML: key: value.
	FormatYAML = sharedoutput.FormatYAML
	// FormatHCL outputs as HCL: key = "value".
	FormatHCL = sharedoutput.FormatHCL
	// FormatEnv outputs as env vars: key=value (GitHub Actions style).
	FormatEnv = sharedoutput.FormatEnv
	// FormatDotenv outputs as dotenv: key='value'.
	FormatDotenv = sharedoutput.FormatDotenv
	// FormatBash outputs as bash exports: export key='value'.
	FormatBash = sharedoutput.FormatBash
	// FormatCSV outputs as CSV: key,value.
	FormatCSV = sharedoutput.FormatCSV
	// FormatTSV outputs as TSV: key<tab>value.
	FormatTSV = sharedoutput.FormatTSV
	// FormatTable outputs as a styled table with Key/Value columns.
	FormatTable = sharedoutput.FormatTable
	// FormatGitHub outputs for GitHub Actions $GITHUB_OUTPUT file.
	FormatGitHub = sharedoutput.FormatGitHub
)

// SupportedFormats lists all supported output formats.
var SupportedFormats = sharedoutput.SupportedFormats

// DefaultFileMode is the file mode for output files.
const DefaultFileMode = sharedoutput.DefaultFileMode

// ScalarOnlyFormats are formats that only support scalar values (not maps/lists).
var ScalarOnlyFormats = sharedoutput.ScalarOnlyFormats

// FormatOptions provides options for output formatting.
type FormatOptions = sharedoutput.FormatOptions

// DefaultFlattenSeparator is the default separator for flattening nested keys.
const DefaultFlattenSeparator = sharedoutput.DefaultFlattenSeparator

// IsComplexValue returns true if the value is a map or slice (not a scalar).
func IsComplexValue(value any) bool {
	return sharedoutput.IsComplexValue(value)
}

// ValidateSingleValueFormat checks if a format supports a single complex value.
func ValidateSingleValueFormat(value any, format Format) error {
	return sharedoutput.ValidateSingleValueFormat(value, format)
}

// WriteToFile writes content to a file, creating it if it doesn't exist
// or appending to it if it does.
func WriteToFile(filePath string, content string) error {
	return sharedoutput.WriteToFile(filePath, content)
}
