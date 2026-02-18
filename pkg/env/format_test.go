package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestFormatData(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		format   Format
		opts     []Option
		expected string
	}{
		{
			name:     "env format with simple values",
			data:     map[string]any{"KEY1": "value1", "KEY2": "value2"},
			format:   FormatEnv,
			expected: "KEY1=value1\nKEY2=value2\n",
		},
		{
			name:     "dotenv format with simple values",
			data:     map[string]any{"KEY1": "value1", "KEY2": "value2"},
			format:   FormatDotenv,
			expected: "KEY1=value1\nKEY2=value2\n",
		},
		{
			name:     "bash format with simple values",
			data:     map[string]any{"KEY1": "value1", "KEY2": "value2"},
			format:   FormatBash,
			expected: "export KEY1=value1\nexport KEY2=value2\n",
		},
		{
			name:     "github format with simple values",
			data:     map[string]any{"KEY1": "value1", "KEY2": "value2"},
			format:   FormatGitHub,
			expected: "KEY1=value1\nKEY2=value2\n",
		},
		{
			name:     "github format with multiline value",
			data:     map[string]any{"CONFIG": "line1\nline2\nline3"},
			format:   FormatGitHub,
			expected: "CONFIG<<ATMOS_EOF_CONFIG\nline1\nline2\nline3\nATMOS_EOF_CONFIG\n",
		},
		{
			name:     "dotenv format with single quotes",
			data:     map[string]any{"MSG": "it's working"},
			format:   FormatDotenv,
			expected: "MSG='it'\"'\"'s working'\n",
		},
		{
			name:     "bash format with single quotes",
			data:     map[string]any{"MSG": "it's working"},
			format:   FormatBash,
			expected: "export MSG='it'\"'\"'s working'\n",
		},
		{
			name:     "env format with boolean",
			data:     map[string]any{"ENABLED": true, "DISABLED": false},
			format:   FormatEnv,
			expected: "DISABLED=false\nENABLED=true\n",
		},
		{
			name:     "env format with integer",
			data:     map[string]any{"COUNT": 42, "PORT": 8080},
			format:   FormatEnv,
			expected: "COUNT=42\nPORT=8080\n",
		},
		{
			name:     "env format with map (JSON encoded)",
			data:     map[string]any{"CONFIG": map[string]any{"key": "value"}},
			format:   FormatEnv,
			expected: "CONFIG={\"key\":\"value\"}\n",
		},
		{
			name:     "env format with slice (JSON encoded)",
			data:     map[string]any{"ITEMS": []any{"a", "b", "c"}},
			format:   FormatEnv,
			expected: "ITEMS=[\"a\",\"b\",\"c\"]\n",
		},
		{
			name:     "github format with JSON multiline",
			data:     map[string]any{"CONFIG": map[string]any{"key": "value", "nested": map[string]any{"a": 1}}},
			format:   FormatGitHub,
			expected: "CONFIG={\"key\":\"value\",\"nested\":{\"a\":1}}\n",
		},
		{
			name:     "with uppercase option",
			data:     map[string]any{"myKey": "value"},
			format:   FormatEnv,
			opts:     []Option{WithUppercase()},
			expected: "MYKEY=value\n",
		},
		{
			name:     "with flatten option",
			data:     map[string]any{"parent": map[string]any{"child": "value"}},
			format:   FormatEnv,
			opts:     []Option{WithFlatten("_")},
			expected: "parent_child=value\n",
		},
		{
			name:     "with uppercase and flatten options",
			data:     map[string]any{"parent": map[string]any{"child": "value"}},
			format:   FormatEnv,
			opts:     []Option{WithUppercase(), WithFlatten("_")},
			expected: "PARENT_CHILD=value\n",
		},
		{
			name:     "nil values are skipped",
			data:     map[string]any{"KEY1": "value1", "KEY2": nil, "KEY3": "value3"},
			format:   FormatEnv,
			expected: "KEY1=value1\nKEY3=value3\n",
		},
		{
			name:     "empty data",
			data:     map[string]any{},
			format:   FormatEnv,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatData(tt.data, tt.format, tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    any
		format   Format
		opts     []Option
		expected string
	}{
		{
			name:     "env format single value",
			key:      "KEY",
			value:    "value",
			format:   FormatEnv,
			expected: "KEY=value\n",
		},
		{
			name:     "bash format single value",
			key:      "KEY",
			value:    "value",
			format:   FormatBash,
			expected: "export KEY=value\n",
		},
		{
			name:     "with uppercase option",
			key:      "myKey",
			value:    "value",
			format:   FormatEnv,
			opts:     []Option{WithUppercase()},
			expected: "MYKEY=value\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatValue(tt.key, tt.value, tt.format, tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValueToString(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{
			name:     "string value",
			value:    "hello",
			expected: "hello",
		},
		{
			name:     "boolean true",
			value:    true,
			expected: "true",
		},
		{
			name:     "boolean false",
			value:    false,
			expected: "false",
		},
		{
			name:     "integer",
			value:    42,
			expected: "42",
		},
		{
			name:     "int64",
			value:    int64(1234567890),
			expected: "1234567890",
		},
		{
			name:     "float32",
			value:    float32(3.14),
			expected: "3.14",
		},
		{
			name:     "float32 whole number",
			value:    float32(42),
			expected: "42",
		},
		{
			name:     "float64",
			value:    3.14,
			expected: "3.14",
		},
		{
			name:     "float64 whole number",
			value:    float64(42),
			expected: "42",
		},
		{
			name:     "map",
			value:    map[string]any{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "slice",
			value:    []any{"a", "b", "c"},
			expected: `["a","b","c"]`,
		},
		{
			name:     "nil",
			value:    nil,
			expected: "",
		},
		{
			name:     "string slice",
			value:    []string{"a", "b"},
			expected: `["a","b"]`,
		},
		{
			name:     "string map",
			value:    map[string]string{"key": "value"},
			expected: `{"key":"value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValueToString(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestValueToString_JSONMarshalError tests the fallback when JSON marshaling fails.
func TestValueToString_JSONMarshalError(t *testing.T) {
	// Channels cannot be marshaled to JSON.
	ch := make(chan int)
	result := ValueToString(ch)

	// Should fall back to %v format.
	assert.Contains(t, result, "0x") // Channels are formatted as pointers.
}

func TestWriteToFile(t *testing.T) {
	t.Run("creates new file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")

		err := WriteToFile(path, "content1\n")
		require.NoError(t, err)

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "content1\n", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")

		err := WriteToFile(path, "content1\n")
		require.NoError(t, err)

		err = WriteToFile(path, "content2\n")
		require.NoError(t, err)

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "content1\ncontent2\n", string(content))
	})

	t.Run("error on invalid path", func(t *testing.T) {
		err := WriteToFile("/nonexistent/dir/file.txt", "content")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})
}

// GitHub helper tests are in pkg/github/actions/env/env_test.go.

func TestUnsupportedFormat(t *testing.T) {
	_, err := FormatData(map[string]any{"key": "value"}, Format("invalid"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
}

func TestFlattenNestedMaps(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		opts     []Option
		expected string
	}{
		{
			name: "deeply nested map",
			data: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "value",
					},
				},
			},
			opts:     []Option{WithFlatten("_")},
			expected: "level1_level2_level3=value\n",
		},
		{
			name: "mixed nested and flat",
			data: map[string]any{
				"flat":   "value1",
				"nested": map[string]any{"child": "value2"},
			},
			opts:     []Option{WithFlatten("_")},
			expected: "flat=value1\nnested_child=value2\n",
		},
		{
			name: "custom separator",
			data: map[string]any{
				"parent": map[string]any{"child": "value"},
			},
			opts:     []Option{WithFlatten("__")},
			expected: "parent__child=value\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatData(tt.data, FormatEnv, tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWithExport(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		opts     []Option
		expected string
	}{
		{
			name:     "bash format default includes export",
			data:     map[string]any{"KEY": "value"},
			opts:     nil,
			expected: "export KEY=value\n",
		},
		{
			name:     "bash format with export=true",
			data:     map[string]any{"KEY": "value"},
			opts:     []Option{WithExport(true)},
			expected: "export KEY=value\n",
		},
		{
			name:     "bash format with export=false",
			data:     map[string]any{"KEY": "value"},
			opts:     []Option{WithExport(false)},
			expected: "KEY=value\n",
		},
		{
			name:     "bash format with export=false and single quotes",
			data:     map[string]any{"MSG": "it's working"},
			opts:     []Option{WithExport(false)},
			expected: "MSG='it'\"'\"'s working'\n",
		},
		{
			name:     "bash format with export=false and multiple keys",
			data:     map[string]any{"KEY1": "value1", "KEY2": "value2"},
			opts:     []Option{WithExport(false)},
			expected: "KEY1=value1\nKEY2=value2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatData(tt.data, FormatBash, tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    Format
		expectError bool
	}{
		{
			name:     "env format",
			input:    "env",
			expected: FormatEnv,
		},
		{
			name:     "dotenv format",
			input:    "dotenv",
			expected: FormatDotenv,
		},
		{
			name:     "bash format",
			input:    "bash",
			expected: FormatBash,
		},
		{
			name:     "github format",
			input:    "github",
			expected: FormatGitHub,
		},
		{
			name:        "invalid format",
			input:       "invalid",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseFormat(tt.input)
			if tt.expectError {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidFormat)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
