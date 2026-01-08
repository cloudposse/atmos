// Package output provides functionality to format and export Terraform outputs
// in various formats suitable for CI/CD workflows, scripts, and configuration files.
package output

import (
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Format represents output format types.
type Format string

const (
	// FormatJSON outputs as JSON object: {"key": "value"}.
	FormatJSON Format = "json"
	// FormatYAML outputs as YAML: key: value.
	FormatYAML Format = "yaml"
	// FormatHCL outputs as HCL: key = "value".
	FormatHCL Format = "hcl"
	// FormatEnv outputs as env vars: key=value (GitHub Actions style).
	FormatEnv Format = "env"
	// FormatDotenv outputs as dotenv: key='value'.
	FormatDotenv Format = "dotenv"
	// FormatBash outputs as bash exports: export key='value'.
	FormatBash Format = "bash"
	// FormatCSV outputs as CSV: key,value.
	FormatCSV Format = "csv"
	// FormatTSV outputs as TSV: key<tab>value.
	FormatTSV Format = "tsv"
)

// SupportedFormats lists all supported output formats.
var SupportedFormats = []string{"json", "yaml", "hcl", "env", "dotenv", "bash", "csv", "tsv"}

// DefaultFileMode is the file mode for output files.
const DefaultFileMode = 0o644

// ScalarOnlyFormats are formats that only support scalar values (not maps/lists).
var ScalarOnlyFormats = []Format{FormatCSV, FormatTSV, FormatEnv, FormatDotenv, FormatBash}

// FormatOptions provides options for output formatting.
type FormatOptions struct {
	// Uppercase converts keys to uppercase (useful for environment variables).
	Uppercase bool
	// Flatten recursively expands nested maps and arrays into flat key/value pairs.
	// For example, {"config": {"host": "localhost"}} becomes {"config_host": "localhost"}.
	// Arrays are flattened with numeric indices: {"hosts": ["a", "b"]} becomes {"hosts_0": "a", "hosts_1": "b"}.
	Flatten bool
	// FlattenSeparator is the separator used when flattening nested keys (default: "_").
	FlattenSeparator string
}

// DefaultFlattenSeparator is the default separator for flattening nested keys.
const DefaultFlattenSeparator = "_"

// GetFlattenSeparator returns the flatten separator, using the default if not set.
func (o FormatOptions) GetFlattenSeparator() string {
	defer perf.Track(nil, "output.FormatOptions.GetFlattenSeparator")()

	if o.FlattenSeparator == "" {
		return DefaultFlattenSeparator
	}
	return o.FlattenSeparator
}

// IsComplexValue returns true if the value is a map or slice (not a scalar).
func IsComplexValue(value any) bool {
	defer perf.Track(nil, "output.IsComplexValue")()

	switch value.(type) {
	case map[string]any, []any:
		return true
	default:
		return false
	}
}

// ValidateSingleValueFormat checks if a format supports a single complex value.
// CSV/TSV formats do not support single complex values (maps/lists).
func ValidateSingleValueFormat(value any, format Format) error {
	defer perf.Track(nil, "output.ValidateSingleValueFormat")()

	if !IsComplexValue(value) {
		return nil
	}

	for _, f := range ScalarOnlyFormats {
		if format == f {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithExplanationf("Format %q does not support complex values (maps/lists) for single output.", format).
				WithHint("Use --format=json or --format=yaml for complex values.").
				Err()
		}
	}
	return nil
}

// WriteToFile writes content to a file, creating it if it doesn't exist
// or appending to it if it does.
func WriteToFile(filePath string, content string) error {
	defer perf.Track(nil, "output.WriteToFile")()

	// Open file in append mode, create if doesn't exist.
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, DefaultFileMode)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}
