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
			name:     "empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name: "single key",
			input: map[string]any{
				"key1": "value1",
			},
			expected: []string{"key1"},
		},
		{
			name: "multiple keys",
			input: map[string]any{
				"key1": "value1",
				"key2": 123,
				"key3": true,
			},
			expected: []string{"key1", "key2", "key3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StringKeysFromMap(tt.input)
			// Sort both slices for comparison since map iteration order is not guaranteed.
			assert.ElementsMatch(t, tt.expected, result)
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
			name:     "key exists",
			m:        map[string]any{"key1": "value1"},
			key:      "key1",
			expected: true,
		},
		{
			name:     "key does not exist",
			m:        map[string]any{"key1": "value1"},
			key:      "key2",
			expected: false,
		},
		{
			name:     "empty map",
			m:        map[string]any{},
			key:      "key1",
			expected: false,
		},
		{
			name:     "key with nil value exists",
			m:        map[string]any{"key1": nil},
			key:      "key1",
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
			name:     "empty map",
			input:    map[string][]string{},
			expected: map[string][]string{},
		},
		{
			name: "single entry no duplicates",
			input: map[string][]string{
				"key1": {"value1", "value2"},
			},
			expected: map[string][]string{
				"key1": {"value1", "value2"},
			},
		},
		{
			name: "multiple entries with duplicates",
			input: map[string][]string{
				"key2": {"b", "a", "b"},
				"key1": {"x", "y", "x"},
			},
			expected: map[string][]string{
				"key1": {"x", "y"},
				"key2": {"a", "b"},
			},
		},
		{
			name: "unsorted values",
			input: map[string][]string{
				"key1": {"c", "a", "b"},
			},
			expected: map[string][]string{
				"key1": {"a", "b", "c"},
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
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]string{},
		},
		{
			name: "all string values",
			input: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "mixed types convertible to string",
			input: map[string]any{
				"key1": "value1",
				"key2": 123,
				"key3": true,
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "123",
				"key3": "true",
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
			name:     "empty map",
			input:    map[any]any{},
			expected: map[string]any{},
		},
		{
			name: "all string keys",
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
			name: "mixed key types - only string keys included",
			input: map[any]any{
				"key1": "value1",
				123:    "value2", // Non-string key will be excluded
			},
			expected: map[string]any{
				"key1": "value1",
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
