package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSliceContainsString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{
			name:     "string exists in slice",
			slice:    []string{"apple", "banana", "cherry"},
			str:      "banana",
			expected: true,
		},
		{
			name:     "string not in slice",
			slice:    []string{"apple", "banana", "cherry"},
			str:      "orange",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			str:      "anything",
			expected: false,
		},
		{
			name:     "empty string in slice",
			slice:    []string{"", "non-empty"},
			str:      "",
			expected: true,
		},
		{
			name:     "nil slice",
			slice:    nil,
			str:      "test",
			expected: false,
		},
		{
			name:     "single element match",
			slice:    []string{"single"},
			str:      "single",
			expected: true,
		},
		{
			name:     "case sensitive check",
			slice:    []string{"Apple", "Banana"},
			str:      "apple",
			expected: false,
		},
		{
			name:     "duplicate values in slice",
			slice:    []string{"dup", "dup", "other"},
			str:      "dup",
			expected: true,
		},
		{
			name:     "special characters",
			slice:    []string{"hello\nworld", "tab\ttab", "space space"},
			str:      "tab\ttab",
			expected: true,
		},
		{
			name:     "unicode strings",
			slice:    []string{"こんにちは", "你好", "hello"},
			str:      "你好",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceContainsString(tt.slice, tt.str)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceContainsInt(t *testing.T) {
	tests := []struct {
		name     string
		slice    []int
		value    int
		expected bool
	}{
		{
			name:     "int exists in slice",
			slice:    []int{1, 2, 3, 4, 5},
			value:    3,
			expected: true,
		},
		{
			name:     "int not in slice",
			slice:    []int{1, 2, 3, 4, 5},
			value:    6,
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []int{},
			value:    1,
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			value:    0,
			expected: false,
		},
		{
			name:     "zero in slice",
			slice:    []int{-1, 0, 1},
			value:    0,
			expected: true,
		},
		{
			name:     "negative numbers",
			slice:    []int{-10, -5, -1},
			value:    -5,
			expected: true,
		},
		{
			name:     "duplicate values",
			slice:    []int{5, 5, 5, 3},
			value:    5,
			expected: true,
		},
		{
			name:     "single element match",
			slice:    []int{42},
			value:    42,
			expected: true,
		},
		{
			name:     "single element no match",
			slice:    []int{42},
			value:    43,
			expected: false,
		},
		{
			name:     "large numbers",
			slice:    []int{1000000, 2000000, 3000000},
			value:    2000000,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceContainsInt(tt.slice, tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceContainsStringStartsWith(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		str      string
		expected bool
	}{
		{
			name:     "string starts with element in slice",
			slice:    []string{"http://", "https://", "ftp://"},
			str:      "http://example.com",
			expected: true,
		},
		{
			name:     "string starts with second element",
			slice:    []string{"http://", "https://", "ftp://"},
			str:      "https://secure.com",
			expected: true,
		},
		{
			name:     "string doesn't start with any element",
			slice:    []string{"http://", "https://"},
			str:      "file://local",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			str:      "anything",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			str:      "test",
			expected: false,
		},
		{
			name:     "empty string in slice",
			slice:    []string{""},
			str:      "anything",
			expected: true, // Everything starts with empty string
		},
		{
			name:     "exact match",
			slice:    []string{"exact"},
			str:      "exact",
			expected: true,
		},
		{
			name:     "case sensitive",
			slice:    []string{"HTTP://"},
			str:      "http://example.com",
			expected: false,
		},
		{
			name:     "substring but not prefix",
			slice:    []string{"example"},
			str:      "myexample",
			expected: false,
		},
		{
			name:     "multiple prefixes match",
			slice:    []string{"te", "test", "testing"},
			str:      "test",
			expected: true, // First match wins
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceContainsStringStartsWith(tt.slice, tt.str)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceContainsStringHasPrefix(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		prefix   string
		expected bool
	}{
		{
			name:     "element has prefix",
			slice:    []string{"http://example.com", "https://secure.com"},
			prefix:   "http://",
			expected: true,
		},
		{
			name:     "multiple elements have prefix",
			slice:    []string{"test1", "test2", "test3"},
			prefix:   "test",
			expected: true,
		},
		{
			name:     "no element has prefix",
			slice:    []string{"apple", "banana", "cherry"},
			prefix:   "orange",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			prefix:   "any",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			prefix:   "test",
			expected: false,
		},
		{
			name:     "empty prefix",
			slice:    []string{"anything", "something"},
			prefix:   "",
			expected: true, // Everything has empty prefix
		},
		{
			name:     "exact match",
			slice:    []string{"exact", "other"},
			prefix:   "exact",
			expected: true,
		},
		{
			name:     "case sensitive",
			slice:    []string{"HTTP://example.com"},
			prefix:   "http://",
			expected: false,
		},
		{
			name:     "substring but not prefix",
			slice:    []string{"myexample", "yourexample"},
			prefix:   "example",
			expected: false,
		},
		{
			name:     "special characters",
			slice:    []string{"/path/to/file", "/path/to/dir"},
			prefix:   "/path/",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceContainsStringHasPrefix(tt.slice, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceOfStringsToSpaceSeparatedString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		expected string
	}{
		{
			name:     "multiple strings",
			slice:    []string{"hello", "world", "test"},
			expected: "hello world test",
		},
		{
			name:     "single string",
			slice:    []string{"single"},
			expected: "single",
		},
		{
			name:     "empty slice",
			slice:    []string{},
			expected: "",
		},
		{
			name:     "nil slice",
			slice:    nil,
			expected: "",
		},
		{
			name:     "strings with spaces",
			slice:    []string{"hello world", "foo bar"},
			expected: "hello world foo bar",
		},
		{
			name:     "empty strings in slice",
			slice:    []string{"", "middle", ""},
			expected: " middle ",
		},
		{
			name:     "special characters",
			slice:    []string{"tab\ttab", "newline\nnewline"},
			expected: "tab\ttab newline\nnewline",
		},
		{
			name:     "unicode strings",
			slice:    []string{"こんにちは", "世界"},
			expected: "こんにちは 世界",
		},
		{
			name:     "numbers as strings",
			slice:    []string{"1", "2", "3"},
			expected: "1 2 3",
		},
		{
			name:     "paths",
			slice:    []string{"/usr/bin", "/usr/local/bin", "/opt/bin"},
			expected: "/usr/bin /usr/local/bin /opt/bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceOfStringsToSpaceSeparatedString(tt.slice)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceOfInterfacesToSliceOdStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []any
		expected []string
	}{
		{
			name:     "mixed types",
			input:    []any{"string", 42, 3.14, true},
			expected: []string{"string", "42", "3.14", "true"},
		},
		{
			name:     "all strings",
			input:    []any{"one", "two", "three"},
			expected: []string{"one", "two", "three"},
		},
		{
			name:     "all numbers",
			input:    []any{1, 2, 3, 4, 5},
			expected: []string{"1", "2", "3", "4", "5"},
		},
		{
			name:     "empty slice",
			input:    []any{},
			expected: []string{},
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "nil values",
			input:    []any{nil, "not nil", nil},
			expected: []string{"<nil>", "not nil", "<nil>"},
		},
		{
			name:     "complex types",
			input:    []any{[]int{1, 2, 3}, map[string]int{"key": 1}},
			expected: []string{"[1 2 3]", "map[key:1]"},
		},
		{
			name:     "boolean values",
			input:    []any{true, false, true},
			expected: []string{"true", "false", "true"},
		},
		{
			name:     "float values",
			input:    []any{1.0, 2.5, 3.14159},
			expected: []string{"1", "2.5", "3.14159"},
		},
		{
			name: "struct values",
			input: []any{
				struct{ Name string }{Name: "test"},
			},
			expected: []string{"{test}"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceOfInterfacesToSliceOdStrings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSliceOfInterfacesToSliceOfStringsWithErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       []any
		expected    []string
		expectError bool
		expectPanic bool
	}{
		{
			name:        "all strings",
			input:       []any{"one", "two", "three"},
			expected:    []string{"one", "two", "three"},
			expectError: false,
		},
		{
			name:        "empty slice",
			input:       []any{},
			expected:    []string{},
			expectError: false,
		},
		{
			name:        "nil input",
			input:       nil,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "contains non-string",
			input:       []any{"string", 42},
			expectPanic: true, // This will panic on type assertion
		},
		{
			name:        "contains nil",
			input:       []any{"string", nil},
			expectPanic: true, // This will panic on type assertion
		},
		{
			name:        "single string",
			input:       []any{"single"},
			expected:    []string{"single"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectPanic {
				assert.Panics(t, func() {
					_, _ = SliceOfInterfacesToSliceOfStrings(tt.input)
				})
			} else {
				result, err := SliceOfInterfacesToSliceOfStrings(tt.input)
				
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					assert.Equal(t, tt.expected, result)
				}
			}
		})
	}
}

func TestSliceContainsString_Performance(t *testing.T) {
	// Test with a large slice to ensure function performs well
	largeSlice := make([]string, 10000)
	for i := range largeSlice {
		largeSlice[i] = fmt.Sprintf("item_%d", i)
	}
	
	// Test finding first element (best case)
	assert.True(t, SliceContainsString(largeSlice, "item_0"))
	
	// Test finding last element (worst case)
	assert.True(t, SliceContainsString(largeSlice, "item_9999"))
	
	// Test not finding element
	assert.False(t, SliceContainsString(largeSlice, "not_found"))
}

func TestSliceContainsInt_Performance(t *testing.T) {
	// Test with a large slice
	largeSlice := make([]int, 10000)
	for i := range largeSlice {
		largeSlice[i] = i
	}
	
	// Test finding first element (best case)
	assert.True(t, SliceContainsInt(largeSlice, 0))
	
	// Test finding last element (worst case)  
	assert.True(t, SliceContainsInt(largeSlice, 9999))
	
	// Test not finding element
	assert.False(t, SliceContainsInt(largeSlice, -1))
}