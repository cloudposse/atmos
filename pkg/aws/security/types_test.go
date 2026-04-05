package security

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseOutputFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected OutputFormat
		wantErr  bool
	}{
		{"markdown", "markdown", FormatMarkdown, false},
		{"md alias", "md", FormatMarkdown, false},
		{"empty defaults to markdown", "", FormatMarkdown, false},
		{"json", "json", FormatJSON, false},
		{"yaml", "yaml", FormatYAML, false},
		{"yml alias", "yml", FormatYAML, false},
		{"csv", "csv", FormatCSV, false},
		{"case insensitive", "JSON", FormatJSON, false},
		{"mixed case", "Yaml", FormatYAML, false},
		{"invalid", "xml", "", true},
		{"invalid format", "html", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseOutputFormat(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseOutputFormat_ErrorType(t *testing.T) {
	// Verify that invalid format errors return the correct sentinel.
	invalidFormats := []string{"xml", "html", "text", "pdf", "   "}

	for _, format := range invalidFormats {
		t.Run(format, func(t *testing.T) {
			_, err := ParseOutputFormat(format)
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrAISecurityInvalidFormat),
				"expected ErrAISecurityInvalidFormat, got: %v", err)
		})
	}
}
