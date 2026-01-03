package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatOptions_GetFlattenSeparator(t *testing.T) {
	tests := []struct {
		name      string
		separator string
		expected  string
	}{
		{
			name:      "default separator when empty",
			separator: "",
			expected:  DefaultFlattenSeparator,
		},
		{
			name:      "custom separator",
			separator: "-",
			expected:  "-",
		},
		{
			name:      "double underscore separator",
			separator: "__",
			expected:  "__",
		},
		{
			name:      "dot separator",
			separator: ".",
			expected:  ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := FormatOptions{FlattenSeparator: tt.separator}
			result := opts.GetFlattenSeparator()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsComplexValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{
			name:     "string is not complex",
			value:    "hello",
			expected: false,
		},
		{
			name:     "int is not complex",
			value:    42,
			expected: false,
		},
		{
			name:     "float is not complex",
			value:    3.14,
			expected: false,
		},
		{
			name:     "bool is not complex",
			value:    true,
			expected: false,
		},
		{
			name:     "nil is not complex",
			value:    nil,
			expected: false,
		},
		{
			name:     "map is complex",
			value:    map[string]any{"key": "value"},
			expected: true,
		},
		{
			name:     "slice is complex",
			value:    []any{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "empty map is complex",
			value:    map[string]any{},
			expected: true,
		},
		{
			name:     "empty slice is complex",
			value:    []any{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsComplexValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateSingleValueFormat(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		format  Format
		wantErr bool
	}{
		{
			name:    "scalar value with env format",
			value:   "hello",
			format:  FormatEnv,
			wantErr: false,
		},
		{
			name:    "scalar value with json format",
			value:   "hello",
			format:  FormatJSON,
			wantErr: false,
		},
		{
			name:    "map value with json format",
			value:   map[string]any{"key": "value"},
			format:  FormatJSON,
			wantErr: false,
		},
		{
			name:    "map value with yaml format",
			value:   map[string]any{"key": "value"},
			format:  FormatYAML,
			wantErr: false,
		},
		{
			name:    "map value with hcl format",
			value:   map[string]any{"key": "value"},
			format:  FormatHCL,
			wantErr: false,
		},
		{
			name:    "map value with env format fails",
			value:   map[string]any{"key": "value"},
			format:  FormatEnv,
			wantErr: true,
		},
		{
			name:    "map value with dotenv format fails",
			value:   map[string]any{"key": "value"},
			format:  FormatDotenv,
			wantErr: true,
		},
		{
			name:    "map value with bash format fails",
			value:   map[string]any{"key": "value"},
			format:  FormatBash,
			wantErr: true,
		},
		{
			name:    "map value with csv format fails",
			value:   map[string]any{"key": "value"},
			format:  FormatCSV,
			wantErr: true,
		},
		{
			name:    "map value with tsv format fails",
			value:   map[string]any{"key": "value"},
			format:  FormatTSV,
			wantErr: true,
		},
		{
			name:    "slice value with csv format fails",
			value:   []any{"a", "b"},
			format:  FormatCSV,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSingleValueFormat(tt.value, tt.format)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWriteToFile(t *testing.T) {
	t.Run("creates new file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test_output.txt")

		err := WriteToFile(filePath, "test content\n")
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "test content\n", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test_append.txt")

		// Write first content.
		err := WriteToFile(filePath, "line1\n")
		require.NoError(t, err)

		// Write second content.
		err = WriteToFile(filePath, "line2\n")
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "line1\nline2\n", string(content))
	})

	t.Run("handles nested directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedDir := filepath.Join(tmpDir, "nested", "dir")
		err := os.MkdirAll(nestedDir, 0o755)
		require.NoError(t, err)

		filePath := filepath.Join(nestedDir, "output.txt")
		err = WriteToFile(filePath, "nested content")
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "nested content", string(content))
	})

	t.Run("fails for invalid path", func(t *testing.T) {
		// Use a path that cannot be created.
		invalidPath := "/nonexistent/path/that/does/not/exist/file.txt"
		err := WriteToFile(invalidPath, "content")
		assert.Error(t, err)
	})
}

func TestSupportedFormats(t *testing.T) {
	// Verify all expected formats are in the list.
	expectedFormats := []string{"json", "yaml", "hcl", "env", "dotenv", "bash", "csv", "tsv"}
	assert.Equal(t, expectedFormats, SupportedFormats)
}

func TestScalarOnlyFormats(t *testing.T) {
	// Verify scalar-only formats are correctly defined.
	expectedScalarFormats := []Format{FormatCSV, FormatTSV, FormatEnv, FormatDotenv, FormatBash}
	assert.Equal(t, expectedScalarFormats, ScalarOnlyFormats)
}

func TestFormatConstants(t *testing.T) {
	// Verify format constants have correct string values.
	assert.Equal(t, Format("json"), FormatJSON)
	assert.Equal(t, Format("yaml"), FormatYAML)
	assert.Equal(t, Format("hcl"), FormatHCL)
	assert.Equal(t, Format("env"), FormatEnv)
	assert.Equal(t, Format("dotenv"), FormatDotenv)
	assert.Equal(t, Format("bash"), FormatBash)
	assert.Equal(t, Format("csv"), FormatCSV)
	assert.Equal(t, Format("tsv"), FormatTSV)
}

func TestDefaultFileMode(t *testing.T) {
	assert.Equal(t, os.FileMode(0o644), os.FileMode(DefaultFileMode))
}
