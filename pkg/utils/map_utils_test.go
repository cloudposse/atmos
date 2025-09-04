package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringKeysFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected []string
	}{
		{
			name: "simple map",
			input: map[string]any{
				"zebra":  "value1",
				"apple":  "value2",
				"banana": "value3",
			},
			expected: []string{"apple", "banana", "zebra"}, // Should be sorted
		},
		{
			name: "map with different value types",
			input: map[string]any{
				"string": "value",
				"int":    42,
				"bool":   true,
				"float":  3.14,
			},
			expected: []string{"bool", "float", "int", "string"},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name: "single key",
			input: map[string]any{
				"only": "one",
			},
			expected: []string{"only"},
		},
		{
			name: "numeric string keys",
			input: map[string]any{
				"10": "ten",
				"2":  "two",
				"1":  "one",
			},
			expected: []string{"1", "10", "2"}, // String sort, not numeric
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringKeysFromMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapKeyExists(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected bool
	}{
		{
			name: "key exists",
			m: map[string]any{
				"existing": "value",
			},
			key:      "existing",
			expected: true,
		},
		{
			name: "key does not exist",
			m: map[string]any{
				"other": "value",
			},
			key:      "missing",
			expected: false,
		},
		{
			name:     "empty map",
			m:        map[string]any{},
			key:      "any",
			expected: false,
		},
		{
			name: "nil value exists",
			m: map[string]any{
				"nil_key": nil,
			},
			key:      "nil_key",
			expected: true,
		},
		{
			name: "empty string key",
			m: map[string]any{
				"": "empty_key",
			},
			key:      "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapKeyExists(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSortMapByKeysAndValuesUniq(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string][]string
		expected map[string][]string
	}{
		{
			name: "map with duplicate values",
			input: map[string][]string{
				"key1": {"banana", "apple", "banana", "cherry"},
				"key2": {"dog", "cat", "dog", "bird"},
			},
			expected: map[string][]string{
				"key1": {"apple", "banana", "cherry"}, // Unique and sorted
				"key2": {"bird", "cat", "dog"},        // Unique and sorted
			},
		},
		{
			name: "already sorted and unique",
			input: map[string][]string{
				"key1": {"a", "b", "c"},
				"key2": {"x", "y", "z"},
			},
			expected: map[string][]string{
				"key1": {"a", "b", "c"},
				"key2": {"x", "y", "z"},
			},
		},
		{
			name:     "empty map",
			input:    map[string][]string{},
			expected: map[string][]string{},
		},
		{
			name: "single value lists",
			input: map[string][]string{
				"key1": {"single"},
				"key2": {"value"},
			},
			expected: map[string][]string{
				"key1": {"single"},
				"key2": {"value"},
			},
		},
		{
			name: "empty value lists",
			input: map[string][]string{
				"key1": {},
				"key2": {},
			},
			expected: map[string][]string{
				"key1": {},
				"key2": {},
			},
		},
		{
			name: "unsorted keys",
			input: map[string][]string{
				"zebra": {"1", "2"},
				"apple": {"3", "4"},
				"mango": {"5", "6"},
			},
			expected: map[string][]string{
				"apple": {"3", "4"},
				"mango": {"5", "6"},
				"zebra": {"1", "2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SortMapByKeysAndValuesUniq(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapOfInterfacesToMapOfStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]string
	}{
		{
			name: "mixed types",
			input: map[string]any{
				"string": "hello",
				"int":    42,
				"float":  3.14,
				"bool":   true,
			},
			expected: map[string]string{
				"string": "hello",
				"int":    "42",
				"float":  "3.14",
				"bool":   "true",
			},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]string{},
		},
		{
			name: "nil values",
			input: map[string]any{
				"nil_value": nil,
				"string":    "not_nil",
			},
			expected: map[string]string{
				"nil_value": "<nil>",
				"string":    "not_nil",
			},
		},
		{
			name: "nested structures",
			input: map[string]any{
				"slice": []int{1, 2, 3},
				"map":   map[string]int{"nested": 1},
			},
			expected: map[string]string{
				"slice": "[1 2 3]",
				"map":   "map[nested:1]",
			},
		},
		{
			name: "special characters",
			input: map[string]any{
				"special": "hello\nworld\t!",
				"unicode": "こんにちは",
			},
			expected: map[string]string{
				"special": "hello\nworld\t!",
				"unicode": "こんにちは",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapOfInterfacesToMapOfStrings(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapOfInterfaceKeysToMapOfStringKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[any]any
		expected map[string]any
	}{
		{
			name: "all string keys",
			input: map[any]any{
				"key1": "value1",
				"key2": "value2",
				"key3": 123,
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": "value2",
				"key3": 123,
			},
		},
		{
			name: "mixed key types",
			input: map[any]any{
				"string_key": "value1",
				123:          "value2", // Non-string key, should be skipped
				true:         "value3", // Non-string key, should be skipped
			},
			expected: map[string]any{
				"string_key": "value1",
			},
		},
		{
			name:     "empty map",
			input:    map[any]any{},
			expected: map[string]any{},
		},
		{
			name: "only non-string keys",
			input: map[any]any{
				123:  "value1",
				true: "value2",
				3.14: "value3",
			},
			expected: map[string]any{},
		},
		{
			name: "nil values",
			input: map[any]any{
				"key1": nil,
				"key2": "not_nil",
			},
			expected: map[string]any{
				"key1": nil,
				"key2": "not_nil",
			},
		},
		{
			name: "complex values",
			input: map[any]any{
				"slice":  []int{1, 2, 3},
				"map":    map[string]int{"nested": 1},
				"struct": struct{ Name string }{Name: "test"},
			},
			expected: map[string]any{
				"slice":  []int{1, 2, 3},
				"map":    map[string]int{"nested": 1},
				"struct": struct{ Name string }{Name: "test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapOfInterfaceKeysToMapOfStringKeys(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapKeyExistsWithNilMap(t *testing.T) {
	// Special test for nil map behavior
	var nilMap map[string]any
	result := MapKeyExists(nilMap, "any_key")
	assert.False(t, result, "nil map should return false for any key")
}

func TestStringKeysFromMapStability(t *testing.T) {
	// Test that the function returns consistent sorted results
	input := map[string]any{
		"z": 1,
		"a": 2,
		"m": 3,
		"b": 4,
		"y": 5,
	}

	// Run multiple times to ensure stability
	var previousResult []string
	for i := 0; i < 10; i++ {
		result := StringKeysFromMap(input)
		if i > 0 {
			assert.Equal(t, previousResult, result, "Results should be consistent across multiple calls")
		}
		previousResult = result
	}
}
