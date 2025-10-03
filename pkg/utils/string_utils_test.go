package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitStringByDelimiter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		delimiter rune
		expected  []string
		expectErr bool
	}{
		{
			name:      "Simple split by space",
			input:     "foo bar baz",
			delimiter: ' ',
			expected:  []string{"foo", "bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Split with quoted sections",
			input:     `"foo bar" baz`,
			delimiter: ' ',
			expected:  []string{"foo bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Empty input string",
			input:     "",
			delimiter: ' ',
			expected:  []string{},
			expectErr: true,
		},
		{
			name:      "Delimiter not present",
			input:     "foobar",
			delimiter: ',',
			expected:  []string{"foobar"},
			expectErr: false,
		},
		{
			name:      "Multiple spaces as delimiter",
			input:     "foo: !env      FOO",
			delimiter: ' ',
			expected:  []string{"foo:", "!env", "FOO"},
			expectErr: false,
		},

		{
			name:      "Error case with invalid CSV format",
			input:     `"foo,bar`,
			delimiter: ',',
			expected:  nil,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SplitStringByDelimiter(tt.input, tt.delimiter)
			if (err != nil) != tt.expectErr {
				t.Errorf("expected error: %v, got: %v", tt.expectErr, err)
			}
			if !tt.expectErr && !equalSlices(t, result, tt.expected) {
				t.Errorf("expected: %v, got: %v", tt.expected, result)
			}
		})
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "all duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "single element",
			input:    []string{"a"},
			expected: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UniqueStrings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitStringAtFirstOccurrence(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		separator string
		expected  [2]string
	}{
		{
			name:      "separator present",
			input:     "key=value",
			separator: "=",
			expected:  [2]string{"key", "value"},
		},
		{
			name:      "separator at beginning",
			input:     "=value",
			separator: "=",
			expected:  [2]string{"", "value"},
		},
		{
			name:      "separator at end",
			input:     "key=",
			separator: "=",
			expected:  [2]string{"key", ""},
		},
		{
			name:      "multiple separators",
			input:     "key=value=extra",
			separator: "=",
			expected:  [2]string{"key", "value=extra"},
		},
		{
			name:      "separator not present",
			input:     "keyvalue",
			separator: "=",
			expected:  [2]string{"keyvalue", ""},
		},
		{
			name:      "empty string",
			input:     "",
			separator: "=",
			expected:  [2]string{"", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitStringAtFirstOccurrence(tt.input, tt.separator)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func equalSlices(t *testing.T, a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			t.Logf("mismatch at index %d: expected %s, got %s", i, b[i], a[i])
			return false
		}
	}
	return true
}
