package format

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTryExpandScalarArray(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "String array",
			input:    []interface{}{"us-east-1a", "us-east-1b", "us-east-1c"},
			expected: "us-east-1a\nus-east-1b\nus-east-1c",
		},
		{
			name:     "Integer array",
			input:    []interface{}{1, 2, 3},
			expected: "1\n2\n3",
		},
		{
			name:     "Boolean array",
			input:    []interface{}{true, false, true},
			expected: "true\nfalse\ntrue",
		},
		{
			name:     "Empty array",
			input:    []interface{}{},
			expected: "",
		},
		{
			name:     "Mixed types (non-scalar)",
			input:    []interface{}{"string", map[string]string{"key": "value"}},
			expected: "", // Should return empty for non-scalar arrays
		},
		{
			name:     "Nested array (non-scalar)",
			input:    []interface{}{[]string{"a", "b"}, []string{"c", "d"}},
			expected: "", // Should return empty for nested arrays
		},
		{
			name:     "Array with very long strings (too wide)",
			input:    []interface{}{"this-is-a-very-long-string-that-exceeds-the-width-threshold", "short"},
			expected: "", // Should return empty if any item is too wide
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := reflect.ValueOf(tt.input)
			result := tryExpandScalarArray(v)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatCollectionValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "Scalar string array expands",
			input:    []interface{}{"zone-a", "zone-b", "zone-c"},
			expected: "zone-a\nzone-b\nzone-c",
		},
		{
			name:     "Scalar map expands",
			input:    map[string]interface{}{"key1": "value1", "key2": "value2"},
			expected: "key1: value1\nkey2: value2",
		},
		{
			name:     "Complex map shows placeholder",
			input:    map[string]interface{}{"key1": map[string]string{"nested": "value"}},
			expected: "{...} (1 keys)",
		},
		{
			name:     "Complex array shows placeholder",
			input:    []interface{}{map[string]string{"a": "b"}, map[string]string{"c": "d"}},
			expected: "[...] (2 items)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := reflect.ValueOf(tt.input)
			result := formatCollectionValue(v)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJoinItems(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		expected string
	}{
		{
			name:     "Multiple items",
			items:    []string{"item1", "item2", "item3"},
			expected: "item1\nitem2\nitem3",
		},
		{
			name:     "Single item",
			items:    []string{"item1"},
			expected: "item1",
		},
		{
			name:     "Empty array",
			items:    []string{},
			expected: "",
		},
		{
			name:     "Items with long values get truncated",
			items:    []string{strings.Repeat("a", 70)},
			expected: strings.Repeat("a", 57) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := joinItems(tt.items)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTableCellValueWithArrays(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		contains string
	}{
		{
			name:     "Scalar array expands",
			input:    []interface{}{"us-east-1a", "us-east-1b"},
			contains: "us-east-1a\nus-east-1b",
		},
		{
			name:     "Scalar map expands",
			input:    map[string]interface{}{"key": "value"},
			contains: "key: value",
		},
		{
			name:     "Complex map shows placeholder",
			input:    map[string]interface{}{"key": map[string]string{"nested": "value"}},
			contains: "{...} (1 keys)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTableCellValue(tt.input)
			assert.Contains(t, result, tt.contains)
		})
	}
}

func TestTryExpandScalarMap(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "String map",
			input:    map[string]interface{}{"env": "production", "team": "platform"},
			expected: "env: production\nteam: platform",
		},
		{
			name:     "Mixed scalar types",
			input:    map[string]interface{}{"count": 5, "enabled": true, "name": "test"},
			expected: "count: 5\nenabled: true\nname: test",
		},
		{
			name:     "Empty map",
			input:    map[string]interface{}{},
			expected: "",
		},
		{
			name:     "Nested map (non-scalar)",
			input:    map[string]interface{}{"outer": map[string]string{"inner": "value"}},
			expected: "", // Should return empty for non-scalar values
		},
		{
			name:     "Map with array value (non-scalar)",
			input:    map[string]interface{}{"list": []string{"a", "b"}},
			expected: "", // Should return empty for non-scalar values
		},
		{
			name:     "Map with very long value (too wide)",
			input:    map[string]interface{}{"key": "this-is-a-very-long-string-that-exceeds-the-width-threshold"},
			expected: "", // Should return empty if any item is too wide
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := reflect.ValueOf(tt.input)
			result := tryExpandScalarMap(v)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetMaxLineWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "Single line",
			input:    "hello",
			expected: 5,
		},
		{
			name:     "Multi-line - first longest",
			input:    "hello world\nhi\nbye",
			expected: 11,
		},
		{
			name:     "Multi-line - middle longest",
			input:    "hi\nhello world\nbye",
			expected: 11,
		},
		{
			name:     "Empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "ANSI colored text",
			input:    "\x1b[31mred\x1b[0m\nblue",
			expected: 4, // "blue" is longer visually, ANSI codes don't count
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMaxLineWidth(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Single line",
			input:    "hello",
			expected: []string{"hello"},
		},
		{
			name:     "Multiple lines",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Trailing newline",
			input:    "line1\nline2\n",
			expected: []string{"line1", "line2", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderInlineMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string // Check if output contains this (for rendered markdown)
	}{
		{
			name:     "Empty string",
			input:    "",
			contains: "",
		},
		{
			name:     "Plain text",
			input:    "Virtual Private Cloud with subnets",
			contains: "Virtual Private Cloud with subnets",
		},
		{
			name:     "Bold text",
			input:    "**Important** configuration",
			contains: "Important",
		},
		{
			name:     "Italic text",
			input:    "*Enhanced* security",
			contains: "Enhanced",
		},
		{
			name:     "Inline code",
			input:    "Configure `vpc_id` parameter",
			contains: "vpc_id",
		},
		{
			name:     "Multiple newlines collapsed",
			input:    "Line one\n\nLine two\n\nLine three",
			contains: "Line one Line two Line three",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderInlineMarkdown(tt.input)
			if tt.contains != "" {
				assert.Contains(t, result, tt.contains)
			} else {
				assert.Equal(t, tt.contains, result)
			}
		})
	}
}

func TestDetectContentType_NoValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected cellContentType
	}{
		{
			name:     "<no value> detected",
			input:    "<no value>",
			expected: contentTypeNoValue,
		},
		{
			name:     "Empty string is default",
			input:    "",
			expected: contentTypeDefault,
		},
		{
			name:     "Boolean true",
			input:    "true",
			expected: contentTypeBoolean,
		},
		{
			name:     "Number",
			input:    "42",
			expected: contentTypeNumber,
		},
		{
			name:     "Placeholder map",
			input:    "{...} (3 keys)",
			expected: contentTypePlaceholder,
		},
		{
			name:     "Placeholder array",
			input:    "[...] (5 items)",
			expected: contentTypePlaceholder,
		},
		{
			name:     "Regular text",
			input:    "vpc",
			expected: contentTypeDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectContentType(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
