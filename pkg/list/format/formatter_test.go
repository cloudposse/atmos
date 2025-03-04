package format

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
)

func TestJSONFormatter(t *testing.T) {
	formatter := &JSONFormatter{}
	data := map[string]interface{}{
		"stack1": map[string]interface{}{
			"value": "test-value",
		},
	}
	options := FormatOptions{Format: FormatJSON}

	output, err := formatter.Format(data, options)
	assert.NoError(t, err)

	// Verify JSON output
	var result map[string]interface{}
	err = json.Unmarshal([]byte(output), &result)
	assert.NoError(t, err)
	assert.Equal(t, data, result)
}

// TestJSONFormatterError tests the error handling in the JSON formatter.
func TestJSONFormatterError(t *testing.T) {
	formatter := &JSONFormatter{}

	data := map[string]interface{}{
		"bad_value": make(chan int),
	}
	options := FormatOptions{Format: FormatJSON}

	output, err := formatter.Format(data, options)
	assert.Error(t, err)
	assert.Empty(t, output)
	assert.Contains(t, err.Error(), "error formatting JSON output")
}

func TestYAMLFormatter(t *testing.T) {
	formatter := &YAMLFormatter{}
	data := map[string]interface{}{
		"stack1": map[string]interface{}{
			"value": "test-value",
		},
	}
	options := FormatOptions{Format: FormatYAML}

	output, err := formatter.Format(data, options)
	assert.NoError(t, err)
	assert.Contains(t, output, "stack1:")
	assert.Contains(t, output, "value: test-value")
}

func TestDelimitedFormatter(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		data     map[string]interface{}
		options  FormatOptions
		expected []string
	}{
		{
			name:   "CSV format",
			format: FormatCSV,
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "test-value",
				},
			},
			options: FormatOptions{
				Format:    FormatCSV,
				Delimiter: DefaultCSVDelimiter,
			},
			expected: []string{
				"Key,stack1",
				"stack1,test-value",
			},
		},
		{
			name:   "TSV format",
			format: FormatTSV,
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "test-value",
				},
			},
			options: FormatOptions{
				Format:    FormatTSV,
				Delimiter: DefaultTSVDelimiter,
			},
			expected: []string{
				"Key\tstack1",
				"stack1\ttest-value",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			formatter := &DelimitedFormatter{format: test.format}
			output, err := formatter.Format(test.data, test.options)
			assert.NoError(t, err)

			lines := strings.Split(strings.TrimSpace(output), utils.GetLineEnding())
			assert.Equal(t, test.expected, lines)
		})
	}
}

func TestTableFormatter(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		options  FormatOptions
		expected []string
	}{
		{
			name: "TTY output",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "test-value",
				},
			},
			options: FormatOptions{
				Format: FormatTable,
				TTY:    true,
			},
			expected: []string{
				"Key",
				"stack1",
				"test-value",
			},
		},
		{
			name: "Non-TTY output",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"value": "test-value",
				},
			},
			options: FormatOptions{
				Format:    FormatTable,
				TTY:       false,
				Delimiter: DefaultCSVDelimiter,
			},
			expected: []string{
				"Key,stack1",
				"stack1,test-value",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			formatter := &TableFormatter{}
			output, err := formatter.Format(test.data, test.options)
			assert.NoError(t, err)

			for _, expected := range test.expected {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestNewFormatter(t *testing.T) {
	tests := []struct {
		name        string
		format      Format
		expectError bool
	}{
		{
			name:        "JSON formatter",
			format:      FormatJSON,
			expectError: false,
		},
		{
			name:        "YAML formatter",
			format:      FormatYAML,
			expectError: false,
		},
		{
			name:        "CSV formatter",
			format:      FormatCSV,
			expectError: false,
		},
		{
			name:        "TSV formatter",
			format:      FormatTSV,
			expectError: false,
		},
		{
			name:        "Table formatter",
			format:      FormatTable,
			expectError: false,
		},
		{
			name:        "Invalid formatter",
			format:      "invalid",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			formatter, err := NewFormatter(test.format)

			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, formatter)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, formatter)
		})
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		expectError bool
	}{
		{
			name:        "Valid JSON format",
			format:      string(FormatJSON),
			expectError: false,
		},
		{
			name:        "Valid YAML format",
			format:      string(FormatYAML),
			expectError: false,
		},
		{
			name:        "Valid CSV format",
			format:      string(FormatCSV),
			expectError: false,
		},
		{
			name:        "Valid TSV format",
			format:      string(FormatTSV),
			expectError: false,
		},
		{
			name:        "Valid Table format",
			format:      string(FormatTable),
			expectError: false,
		},
		{
			name:        "Invalid format",
			format:      "invalid",
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateFormat(test.format)

			if test.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
