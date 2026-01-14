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

// TestCalculateColumnWidths tests the column width calculation logic.
func TestCalculateColumnWidths(t *testing.T) {
	tests := []struct {
		name          string
		header        []string
		rows          [][]string
		terminalWidth int
		expectMinLen  int
	}{
		{
			name:          "Empty columns",
			header:        []string{},
			rows:          [][]string{},
			terminalWidth: 120,
			expectMinLen:  0,
		},
		{
			name:          "Simple table",
			header:        []string{"Name", "Value"},
			rows:          [][]string{{"vpc", "10.0.0.0/16"}},
			terminalWidth: 120,
			expectMinLen:  2,
		},
		{
			name:          "Table with Description column",
			header:        []string{"Component", "Stack", "Description"},
			rows:          [][]string{{"vpc", "prod", "Virtual Private Cloud for production"}},
			terminalWidth: 120,
			expectMinLen:  3,
		},
		{
			name:          "Narrow terminal",
			header:        []string{"Name", "Value"},
			rows:          [][]string{{"component", "value"}},
			terminalWidth: 40,
			expectMinLen:  2,
		},
		{
			name:          "Very narrow terminal (minimum width enforcement)",
			header:        []string{"Name", "Value"},
			rows:          [][]string{{"a", "b"}},
			terminalWidth: 10,
			expectMinLen:  2,
		},
		{
			name:          "Multi-line content",
			header:        []string{"Tags"},
			rows:          [][]string{{"env: prod\nteam: platform\nregion: us-east-1"}},
			terminalWidth: 120,
			expectMinLen:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateColumnWidths(tt.header, tt.rows, tt.terminalWidth)
			assert.Equal(t, tt.expectMinLen, len(result))

			// Verify all widths are positive (actual minimum may vary based on terminal width constraints).
			for i, width := range result {
				assert.Greater(t, width, 0, "Column %d width %d should be positive", i, width)
			}
		})
	}
}

// TestCreateStyledTable tests the main table creation function.
func TestCreateStyledTable(t *testing.T) {
	tests := []struct {
		name     string
		header   []string
		rows     [][]string
		contains string
	}{
		{
			name:     "Simple table",
			header:   []string{"Name", "Value"},
			rows:     [][]string{{"vpc", "prod"}},
			contains: "Name",
		},
		{
			name:     "Empty table",
			header:   []string{"Name"},
			rows:     [][]string{},
			contains: "Name",
		},
		{
			name:     "Table with Description column (markdown rendering)",
			header:   []string{"Component", "Description"},
			rows:     [][]string{{"vpc", "**Virtual** Private Cloud"}},
			contains: "Component",
		},
		{
			name:     "Multi-column table",
			header:   []string{"Component", "Stack", "Type", "Enabled"},
			rows:     [][]string{{"vpc", "prod-ue2-dev", "terraform", "true"}},
			contains: "Component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CreateStyledTable(tt.header, tt.rows)
			assert.NotEmpty(t, result)
			assert.Contains(t, result, tt.contains)
		})
	}
}

// TestTableFormatterFormat tests the Format method.
func TestTableFormatterFormat(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]interface{}
		options     FormatOptions
		expectError bool
	}{
		{
			name: "TTY mode with simple data",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vars": map[string]interface{}{
						"environment": "prod",
					},
				},
			},
			options: FormatOptions{
				TTY: true,
			},
			expectError: false,
		},
		{
			name: "Non-TTY mode (falls back to CSV)",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vars": map[string]interface{}{
						"environment": "prod",
					},
				},
			},
			options: FormatOptions{
				TTY: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &TableFormatter{}
			result, err := formatter.Format(tt.data, tt.options)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}
		})
	}
}

// TestPadToWidth tests string padding logic.
func TestPadToWidth(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "No padding needed",
			input:    "hello",
			width:    5,
			expected: "hello",
		},
		{
			name:     "Padding needed",
			input:    "hi",
			width:    10,
			expected: "hi        ",
		},
		{
			name:     "Zero width",
			input:    "test",
			width:    0,
			expected: "test",
		},
		{
			name:     "Negative width",
			input:    "test",
			width:    -1,
			expected: "test",
		},
		{
			name:     "Multi-line content",
			input:    "line1\nline2",
			width:    10,
			expected: "line1     \nline2     ",
		},
		{
			name:     "Already wider than target",
			input:    "very long string",
			width:    5,
			expected: "very long string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padToWidth(tt.input, tt.width)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCreateHeader tests header creation.
func TestCreateHeader(t *testing.T) {
	tests := []struct {
		name          string
		stackKeys     []string
		customHeaders []string
		expected      []string
	}{
		{
			name:          "Default headers",
			stackKeys:     []string{"stack1", "stack2"},
			customHeaders: []string{},
			expected:      []string{"Key", "stack1", "stack2"},
		},
		{
			name:          "Custom headers",
			stackKeys:     []string{"stack1", "stack2"},
			customHeaders: []string{"Name", "Env1", "Env2"},
			expected:      []string{"Name", "Env1", "Env2"},
		},
		{
			name:          "Empty stack keys",
			stackKeys:     []string{},
			customHeaders: []string{},
			expected:      []string{"Key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createHeader(tt.stackKeys, tt.customHeaders)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCreateRows tests row creation.
func TestCreateRows(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		valueKeys []string
		stackKeys []string
		expectLen int
	}{
		{
			name: "Simple key-value rows",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vars": map[string]interface{}{
						"environment": "prod",
						"region":      "us-east-1",
					},
				},
			},
			valueKeys: []string{"environment", "region"},
			stackKeys: []string{"stack1"},
			expectLen: 2,
		},
		{
			name: "Value keyword handling",
			data: map[string]interface{}{
				"stack1": "simple-value",
			},
			valueKeys: []string{"value"},
			stackKeys: []string{"stack1"},
			expectLen: 1,
		},
		{
			name: "Multiple stacks",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vars": map[string]interface{}{
						"environment": "prod",
					},
				},
				"stack2": map[string]interface{}{
					"vars": map[string]interface{}{
						"environment": "dev",
					},
				},
			},
			valueKeys: []string{"environment"},
			stackKeys: []string{"stack1", "stack2"},
			expectLen: 1,
		},
		{
			name:      "Empty data",
			data:      map[string]interface{}{},
			valueKeys: []string{},
			stackKeys: []string{},
			expectLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := createRows(tt.data, tt.valueKeys, tt.stackKeys)
			assert.Equal(t, tt.expectLen, len(result))
		})
	}
}

// TestExtractAndSortKeys tests key extraction and sorting.
func TestExtractAndSortKeys(t *testing.T) {
	tests := []struct {
		name       string
		data       map[string]interface{}
		maxColumns int
		expected   []string
	}{
		{
			name: "Alphabetical sorting",
			data: map[string]interface{}{
				"zebra":  "value",
				"apple":  "value",
				"banana": "value",
			},
			maxColumns: 0,
			expected:   []string{"apple", "banana", "zebra"},
		},
		{
			name: "Max columns limit",
			data: map[string]interface{}{
				"alpha": "value",
				"beta":  "value",
				"gamma": "value",
				"delta": "value",
			},
			maxColumns: 2,
			expected:   []string{"alpha", "beta"},
		},
		{
			name:       "Empty data",
			data:       map[string]interface{}{},
			maxColumns: 0,
			expected:   nil, // extractAndSortKeys returns nil for empty data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractAndSortKeys(tt.data, tt.maxColumns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractValueKeys tests value key extraction.
func TestExtractValueKeys(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		stackKeys []string
		expected  []string
	}{
		{
			name: "Extract from vars",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"vars": map[string]interface{}{
						"environment": "prod",
						"region":      "us-east-1",
					},
				},
			},
			stackKeys: []string{"stack1"},
			expected:  []string{"environment", "region"},
		},
		{
			name: "Extract from top-level keys",
			data: map[string]interface{}{
				"stack1": map[string]interface{}{
					"component": "vpc",
					"stack":     "prod",
				},
			},
			stackKeys: []string{"stack1"},
			expected:  []string{"component", "stack"},
		},
		{
			name: "Array value returns 'value'",
			data: map[string]interface{}{
				"stack1": []interface{}{"item1", "item2"},
			},
			stackKeys: []string{"stack1"},
			expected:  []string{"value"},
		},
		{
			name: "Scalar value returns 'value'",
			data: map[string]interface{}{
				"stack1": "simple-string",
			},
			stackKeys: []string{"stack1"},
			expected:  []string{"value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractValueKeys(tt.data, tt.stackKeys)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

// TestFormatComplexValue tests JSON formatting for complex values.
func TestFormatComplexValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		contains string
	}{
		{
			name:     "Map value",
			input:    map[string]string{"key": "value"},
			contains: "key",
		},
		{
			name:     "Struct value",
			input:    struct{ Name string }{Name: "test"},
			contains: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatComplexValue(tt.input)
			assert.NotEmpty(t, result)
			if tt.contains != "" {
				assert.Contains(t, result, tt.contains)
			}
		})
	}
}

// TestTruncateString tests string truncation.
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Short string (no truncation)",
			input:    "short",
			expected: "short",
		},
		{
			name:     "Exact max width",
			input:    strings.Repeat("a", MaxColumnWidth),
			expected: strings.Repeat("a", MaxColumnWidth),
		},
		{
			name:     "Exceeds max width",
			input:    strings.Repeat("a", MaxColumnWidth+10),
			expected: strings.Repeat("a", MaxColumnWidth-3) + "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatTableCellValue tests cell value formatting.
func TestFormatTableCellValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "Nil value",
			input:    nil,
			expected: "",
		},
		{
			name:     "String value",
			input:    "test-value",
			expected: "test-value",
		},
		{
			name:     "Boolean true",
			input:    true,
			expected: "true",
		},
		{
			name:     "Boolean false",
			input:    false,
			expected: "false",
		},
		{
			name:     "Integer",
			input:    42,
			expected: "42",
		},
		{
			name:     "Float",
			input:    3.14159,
			expected: "3.14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTableCellValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
