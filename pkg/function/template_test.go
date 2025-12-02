package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplateFunction(t *testing.T) {
	fn := NewTemplateFunction()

	assert.Equal(t, "template", fn.Name())
	assert.Empty(t, fn.Aliases())
	assert.Equal(t, PreMerge, fn.Phase())
}

func TestTemplateFunctionExecute(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		expected any
	}{
		{
			name:     "empty args returns empty string",
			args:     "",
			expected: "",
		},
		{
			name:     "whitespace only returns empty string",
			args:     "   ",
			expected: "",
		},
		{
			name:     "valid JSON object",
			args:     `{"key": "value"}`,
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "valid JSON array",
			args:     `[1, 2, 3]`,
			expected: []any{float64(1), float64(2), float64(3)},
		},
		{
			name:     "valid JSON string",
			args:     `"hello"`,
			expected: "hello",
		},
		{
			name:     "valid JSON number",
			args:     `42`,
			expected: float64(42),
		},
		{
			name:     "valid JSON boolean",
			args:     `true`,
			expected: true,
		},
		{
			name:     "valid JSON null",
			args:     `null`,
			expected: nil,
		},
		{
			name:     "invalid JSON returns string",
			args:     `{not valid json`,
			expected: `{not valid json`,
		},
		{
			name:     "plain string returns string",
			args:     `hello world`,
			expected: `hello world`,
		},
		{
			name:     "nested JSON",
			args:     `{"outer": {"inner": [1, 2, 3]}}`,
			expected: map[string]any{"outer": map[string]any{"inner": []any{float64(1), float64(2), float64(3)}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := NewTemplateFunction()

			result, err := fn.Execute(context.Background(), tt.args, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessTemplateTagsOnly(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]any
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]any{},
		},
		{
			name: "no template tags",
			input: map[string]any{
				"key":    "value",
				"number": 42,
			},
			expected: map[string]any{
				"key":    "value",
				"number": 42,
			},
		},
		{
			name: "template tag with JSON object",
			input: map[string]any{
				"config": `!template {"enabled": true}`,
			},
			expected: map[string]any{
				"config": map[string]any{"enabled": true},
			},
		},
		{
			name: "template tag with JSON array",
			input: map[string]any{
				"list": `!template [1, 2, 3]`,
			},
			expected: map[string]any{
				"list": []any{float64(1), float64(2), float64(3)},
			},
		},
		{
			name: "nested template tags",
			input: map[string]any{
				"outer": map[string]any{
					"inner": `!template {"nested": true}`,
				},
			},
			expected: map[string]any{
				"outer": map[string]any{
					"inner": map[string]any{"nested": true},
				},
			},
		},
		{
			name: "template tags in arrays",
			input: map[string]any{
				"items": []any{
					`!template {"id": 1}`,
					`!template {"id": 2}`,
				},
			},
			expected: map[string]any{
				"items": []any{
					map[string]any{"id": float64(1)},
					map[string]any{"id": float64(2)},
				},
			},
		},
		{
			name: "other tags preserved",
			input: map[string]any{
				"env":      "!env HOME",
				"template": `!template {"key": "value"}`,
			},
			expected: map[string]any{
				"env":      "!env HOME",
				"template": map[string]any{"key": "value"},
			},
		},
		{
			name: "mixed types",
			input: map[string]any{
				"string":   "plain",
				"number":   123,
				"bool":     true,
				"template": `!template [1, 2]`,
				"nested": map[string]any{
					"key": `!template {"a": "b"}`,
				},
			},
			expected: map[string]any{
				"string":   "plain",
				"number":   123,
				"bool":     true,
				"template": []any{float64(1), float64(2)},
				"nested": map[string]any{
					"key": map[string]any{"a": "b"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ProcessTemplateTagsOnly(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
