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
			name: "multiple keys",
			input: map[string]any{
				"zebra": 1,
				"alpha": 2,
				"beta":  3,
			},
			expected: []string{"alpha", "beta", "zebra"},
		},
		{
			name: "single key",
			input: map[string]any{
				"key": "value",
			},
			expected: []string{"key"},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: []string{},
		},
		{
			name: "keys with numbers",
			input: map[string]any{
				"key3": 1,
				"key1": 2,
				"key2": 3,
			},
			expected: []string{"key1", "key2", "key3"},
		},
		{
			name: "keys with special characters",
			input: map[string]any{
				"foo-bar": 1,
				"foo_bar": 2,
				"foo.bar": 3,
			},
			expected: []string{"foo-bar", "foo.bar", "foo_bar"},
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
		inputMap map[string]any
		key      string
		expected bool
	}{
		{
			name: "key exists",
			inputMap: map[string]any{
				"foo": "bar",
				"baz": 123,
			},
			key:      "foo",
			expected: true,
		},
		{
			name: "key does not exist",
			inputMap: map[string]any{
				"foo": "bar",
			},
			key:      "missing",
			expected: false,
		},
		{
			name:     "empty map",
			inputMap: map[string]any{},
			key:      "any",
			expected: false,
		},
		{
			name: "key exists with nil value",
			inputMap: map[string]any{
				"foo": nil,
			},
			key:      "foo",
			expected: true,
		},
		{
			name: "key exists with empty string value",
			inputMap: map[string]any{
				"foo": "",
			},
			key:      "foo",
			expected: true,
		},
		{
			name: "key exists with zero value",
			inputMap: map[string]any{
				"foo": 0,
			},
			key:      "foo",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapKeyExists(tt.inputMap, tt.key)
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
			name: "sort keys and deduplicate values",
			input: map[string][]string{
				"zebra": {"c", "a", "b", "a"},
				"alpha": {"z", "x", "y", "x"},
				"beta":  {"3", "1", "2", "1"},
			},
			expected: map[string][]string{
				"alpha": {"x", "y", "z"},
				"beta":  {"1", "2", "3"},
				"zebra": {"a", "b", "c"},
			},
		},
		{
			name: "already sorted and unique",
			input: map[string][]string{
				"a": {"1", "2", "3"},
				"b": {"x", "y", "z"},
			},
			expected: map[string][]string{
				"a": {"1", "2", "3"},
				"b": {"x", "y", "z"},
			},
		},
		{
			name: "all duplicates",
			input: map[string][]string{
				"key": {"a", "a", "a"},
			},
			expected: map[string][]string{
				"key": {"a"},
			},
		},
		{
			name:     "empty map",
			input:    map[string][]string{},
			expected: map[string][]string{},
		},
		{
			name: "empty value slices",
			input: map[string][]string{
				"a": {},
				"b": {},
			},
			expected: map[string][]string{
				"a": {},
				"b": {},
			},
		},
		{
			name: "single value per key",
			input: map[string][]string{
				"z": {"one"},
				"a": {"two"},
			},
			expected: map[string][]string{
				"a": {"two"},
				"z": {"one"},
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
			name: "various types",
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
			name: "all strings",
			input: map[string]any{
				"foo": "bar",
				"baz": "qux",
			},
			expected: map[string]string{
				"foo": "bar",
				"baz": "qux",
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
				"key": nil,
			},
			expected: map[string]string{
				"key": "<nil>",
			},
		},
		{
			name: "zero values",
			input: map[string]any{
				"int":    0,
				"string": "",
				"bool":   false,
			},
			expected: map[string]string{
				"int":    "0",
				"string": "",
				"bool":   "false",
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
				"foo": "bar",
				"baz": 123,
			},
			expected: map[string]any{
				"foo": "bar",
				"baz": 123,
			},
		},
		{
			name: "mixed key types - string keys preserved",
			input: map[any]any{
				"string_key": "value1",
				42:           "value2", // non-string key, should be ignored
				true:         "value3", // non-string key, should be ignored
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
			name: "no string keys",
			input: map[any]any{
				42:   "value1",
				true: "value2",
			},
			expected: map[string]any{},
		},
		{
			name: "string keys with various value types",
			input: map[any]any{
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"nil":    nil,
				"string": "value",
			},
			expected: map[string]any{
				"int":    42,
				"float":  3.14,
				"bool":   true,
				"nil":    nil,
				"string": "value",
			},
		},
		{
			name: "empty string key",
			input: map[any]any{
				"": "empty_key_value",
			},
			expected: map[string]any{
				"": "empty_key_value",
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
