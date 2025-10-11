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
			name:      "Single quoted value with nested double quotes",
			input:     "core '.security.users[\"github-dependabot\"].access.key.id'",
			delimiter: ' ',
			expected:  []string{"core", ".security.users[\"github-dependabot\"].access.key.id"},
			expectErr: false,
		},
		{
			name:      "Single quoted value with escaped single quotes",
			input:     "core '.security.users[''github-dependabot''].access.key.id'",
			delimiter: ' ',
			expected:  []string{"core", ".security.users['github-dependabot'].access.key.id"},
			expectErr: false,
		},
		{
			name: "Double quoted value with escaped double quotes",
			// If the parser sees "" (two consecutive double quotes inside a quoted string), according to CSV/Excel-like
			// conventions, a "" inside quotes means a literal " character in the final value.
			input:     "\"foo\"\"bar\" baz",
			delimiter: ' ',
			expected:  []string{"foo\"bar", "baz"},
			expectErr: false,
		},
		{
			name:      "Quoted empty values are removed",
			input:     "foo '' \"\" bar",
			delimiter: ' ',
			expected:  []string{"foo", "bar"},
			expectErr: false,
		},
		{
			name:      "Unmatched leading quote is preserved",
			input:     "foo 'bar",
			delimiter: ' ',
			expected:  []string{"foo", "'bar"},
			expectErr: false,
		},

		{
			name:      "Error case with invalid CSV format",
			input:     `"foo,bar`,
			delimiter: ',',
			expected:  nil,
			expectErr: true,
		},
		{
			name:      "Bare quote triggers LazyQuotes retry",
			input:     `foo b"ar baz`,
			delimiter: ' ',
			expected:  []string{"foo", "b\"ar", "baz"},
			expectErr: false,
		},
		{
			name:      "Multiple bare quotes with LazyQuotes fallback",
			input:     `a"b c"d`,
			delimiter: ' ',
			expected:  []string{"a\"b", "c\"d"},
			expectErr: false,
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

// TestUniqueStrings tests the UniqueStrings function.
func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "No duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "With duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "Empty slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "All duplicates",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "Nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "Single element",
			input:    []string{"single"},
			expected: []string{"single"},
		},
		{
			name:     "Order preservation - first occurrence kept",
			input:    []string{"third", "first", "second", "first", "third"},
			expected: []string{"third", "first", "second"},
		},
		{
			name:     "Empty strings are preserved",
			input:    []string{"", "a", "", "b"},
			expected: []string{"", "a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UniqueStrings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTrimMatchingQuotes tests the trimMatchingQuotes function.
func TestTrimMatchingQuotes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Double quotes removed",
			input:    `"value"`,
			expected: "value",
		},
		{
			name:     "Single quotes removed",
			input:    "'value'",
			expected: "value",
		},
		{
			name:     "Escaped double quotes normalized",
			input:    `"val""ue"`,
			expected: `val"ue`,
		},
		{
			name:     "Escaped single quotes normalized",
			input:    "'val''ue'",
			expected: "val'ue",
		},
		{
			name:     "Mismatched quotes preserved",
			input:    `"value'`,
			expected: `"value'`,
		},
		{
			name:     "No quotes preserved",
			input:    "value",
			expected: "value",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Single character",
			input:    "a",
			expected: "a",
		},
		{
			name:     "Only quotes",
			input:    `""`,
			expected: "",
		},
		{
			name:     "Leading quote only",
			input:    `"value`,
			expected: `"value`,
		},
		{
			name:     "Trailing quote only",
			input:    `value"`,
			expected: `value"`,
		},
		{
			name:     "Multiple escaped quotes",
			input:    `"a""b""c"`,
			expected: `a"b"c`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := trimMatchingQuotes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSplitStringAtFirstOccurrence tests the SplitStringAtFirstOccurrence function.
func TestSplitStringAtFirstOccurrence(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		separator string
		expected  [2]string
	}{
		{
			name:      "Split with separator present",
			input:     "key=value",
			separator: "=",
			expected:  [2]string{"key", "value"},
		},
		{
			name:      "Split with multiple separators",
			input:     "key=value=extra",
			separator: "=",
			expected:  [2]string{"key", "value=extra"},
		},
		{
			name:      "No separator present",
			input:     "keyvalue",
			separator: "=",
			expected:  [2]string{"keyvalue", ""},
		},
		{
			name:      "Empty string",
			input:     "",
			separator: "=",
			expected:  [2]string{"", ""},
		},
		{
			name:      "Separator at start",
			input:     "=value",
			separator: "=",
			expected:  [2]string{"", "value"},
		},
		{
			name:      "Separator at end",
			input:     "key=",
			separator: "=",
			expected:  [2]string{"key", ""},
		},
		{
			name:      "Multi-character separator",
			input:     "key::value::extra",
			separator: "::",
			expected:  [2]string{"key", "value::extra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitStringAtFirstOccurrence(tt.input, tt.separator)
			assert.Equal(t, tt.expected, result)
		})
	}
}
