package format

import (
	"github.com/cloudposse/atmos/pkg/list/errors"
)

// Format represents the output format type.
type Format string

const (
	FormatTable    Format = "table"
	FormatJSON     Format = "json"
	FormatYAML     Format = "yaml"
	FormatCSV      Format = "csv"
	FormatTSV      Format = "tsv"
	FormatTemplate Format = "template"
	FormatTree     Format = "tree"
)

// FormatOptions contains options for formatting output.
type FormatOptions struct {
	MaxColumns    int
	Delimiter     string
	TTY           bool
	Format        Format
	CustomHeaders []string
}

// Formatter defines the interface for formatting output.
type Formatter interface {
	Format(data map[string]interface{}, options FormatOptions) (string, error)
}

// DefaultFormatter provides a base implementation of Formatter.
type DefaultFormatter struct {
	format Format
}

// TableFormatter handles table format output.
type TableFormatter struct {
	DefaultFormatter
}

// JSONFormatter handles JSON format output.
type JSONFormatter struct {
	DefaultFormatter
}

// YAMLFormatter handles YAML format output.
type YAMLFormatter struct {
	DefaultFormatter
}

// DelimitedFormatter handles CSV and TSV format output.
type DelimitedFormatter struct {
	DefaultFormatter
	format Format
}

// NewFormatter creates a new formatter for the specified format.
func NewFormatter(format Format) (Formatter, error) {
	switch format {
	case FormatTable:
		return &TableFormatter{DefaultFormatter{format: format}}, nil
	case FormatJSON:
		return &JSONFormatter{DefaultFormatter{format: format}}, nil
	case FormatYAML:
		return &YAMLFormatter{DefaultFormatter{format: format}}, nil
	case FormatCSV, FormatTSV:
		return &DelimitedFormatter{DefaultFormatter: DefaultFormatter{format: format}, format: format}, nil
	default:
		return nil, &errors.InvalidFormatError{
			Format: string(format),
			Valid:  []string{string(FormatTable), string(FormatJSON), string(FormatYAML), string(FormatCSV), string(FormatTSV)},
		}
	}
}

// ValidateFormat checks if the provided format is valid.
func ValidateFormat(format string) error {
	validFormats := []Format{FormatTable, FormatJSON, FormatYAML, FormatCSV, FormatTSV, FormatTree}
	for _, f := range validFormats {
		if Format(format) == f {
			return nil
		}
	}
	return &errors.InvalidFormatError{
		Format: format,
		Valid:  []string{string(FormatTable), string(FormatJSON), string(FormatYAML), string(FormatCSV), string(FormatTSV), string(FormatTree)},
	}
}
