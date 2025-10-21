package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStringKeysFromMap tests the StringKeysFromMap function.
func TestStringKeysFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected []string
	}{
		{
			name:     "Map with multiple keys",
			input:    map[string]any{"z": 1, "a": 2, "m": 3},
			expected: []string{"a", "m", "z"},
		},
		{
			name:     "Empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name:     "Single key",
			input:    map[string]any{"key": "value"},
			expected: []string{"key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringKeysFromMap(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMapKeyExists tests the MapKeyExists function.
func TestMapKeyExists(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected bool
	}{
		{
			name:     "Key exists",
			m:        map[string]any{"foo": "bar", "baz": 123},
			key:      "foo",
			expected: true,
		},
		{
			name:     "Key does not exist",
			m:        map[string]any{"foo": "bar"},
			key:      "missing",
			expected: false,
		},
		{
			name:     "Empty map",
			m:        map[string]any{},
			key:      "foo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapKeyExists(tt.m, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSortMapByKeysAndValuesUniq tests the SortMapByKeysAndValuesUniq function.
func TestSortMapByKeysAndValuesUniq(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string][]string
		expected map[string][]string
	}{
		{
			name: "Sort keys and values with duplicates",
			input: map[string][]string{
				"z": {"z", "a", "a", "m"},
				"a": {"x", "y", "x"},
			},
			expected: map[string][]string{
				"a": {"x", "y"},
				"z": {"a", "m", "z"},
			},
		},
		{
			name:     "Empty map",
			input:    map[string][]string{},
			expected: map[string][]string{},
		},
		{
			name: "No duplicates",
			input: map[string][]string{
				"b": {"1", "2"},
				"a": {"3"},
			},
			expected: map[string][]string{
				"a": {"3"},
				"b": {"1", "2"},
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

// TestMapOfInterfacesToMapOfStrings tests the MapOfInterfacesToMapOfStrings function.
func TestMapOfInterfacesToMapOfStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]string
	}{
		{
			name: "Mixed types",
			input: map[string]any{
				"string": "value",
				"int":    123,
				"bool":   true,
			},
			expected: map[string]string{
				"string": "value",
				"int":    "123",
				"bool":   "true",
			},
		},
		{
			name:     "Empty map",
			input:    map[string]any{},
			expected: map[string]string{},
		},
		{
			name: "All strings",
			input: map[string]any{
				"a": "1",
				"b": "2",
			},
			expected: map[string]string{
				"a": "1",
				"b": "2",
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

// TestMapOfInterfaceKeysToMapOfStringKeys tests the MapOfInterfaceKeysToMapOfStringKeys function.
func TestMapOfInterfaceKeysToMapOfStringKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[any]any
		expected map[string]any
	}{
		{
			name: "All string keys",
			input: map[any]any{
				"key1": "value1",
				"key2": 123,
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": 123,
			},
		},
		{
			name: "Mixed key types",
			input: map[any]any{
				"string_key": "value",
				123:          "ignored",
			},
			expected: map[string]any{
				"string_key": "value",
			},
		},
		{
			name:     "Empty map",
			input:    map[any]any{},
			expected: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapOfInterfaceKeysToMapOfStringKeys(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
