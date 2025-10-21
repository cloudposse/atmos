package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "comma separated with spaces",
			input:    "tag1, tag2, tag3",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "comma separated without spaces",
			input:    "tag1,tag2,tag3",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name:     "single item",
			input:    "tag1",
			sep:      ",",
			expected: []string{"tag1"},
		},
		{
			name:     "empty string",
			input:    "",
			sep:      ",",
			expected: []string{""},
		},
		{
			name:     "with extra spaces",
			input:    " tag1 , tag2 , tag3 ",
			sep:      ",",
			expected: []string{"tag1", "tag2", "tag3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitAndTrim(tt.input, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{
			name:     "comma separated",
			input:    "a,b,c",
			sep:      ",",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "pipe separated",
			input:    "a|b|c",
			sep:      "|",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "single item",
			input:    "single",
			sep:      ",",
			expected: []string{"single"},
		},
		{
			name:     "empty string",
			input:    "",
			sep:      ",",
			expected: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitString(tt.input, tt.sep)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTrimString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "string with leading and trailing spaces",
			input:    "  hello  ",
			expected: "hello",
		},
		{
			name:     "string with only leading spaces",
			input:    "  hello",
			expected: "hello",
		},
		{
			name:     "string with only trailing spaces",
			input:    "hello  ",
			expected: "hello",
		},
		{
			name:     "string without spaces",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "string with tabs and newlines",
			input:    "\t\nhello\n\t",
			expected: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
