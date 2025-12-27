package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAppendTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected bool
	}{
		{
			name:     "append tag",
			tag:      "!append",
			expected: true,
		},
		{
			name:     "other tag",
			tag:      "!include",
			expected: false,
		},
		{
			name:     "empty tag",
			tag:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAppendTag(tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasAppendTag(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected bool
	}{
		{
			name: "has append tag",
			value: map[string]any{
				"__atmos_append__": []any{"item1", "item2"},
			},
			expected: true,
		},
		{
			name: "no append tag",
			value: map[string]any{
				"regular": "value",
			},
			expected: false,
		},
		{
			name:     "not a map",
			value:    []any{"item1", "item2"},
			expected: false,
		},
		{
			name:     "nil value",
			value:    nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasAppendTag(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractAppendListValue(t *testing.T) {
	tests := []struct {
		name         string
		value        any
		expectedList []any
		expectedOk   bool
	}{
		{
			name: "valid append tag",
			value: map[string]any{
				"__atmos_append__": []any{"item1", "item2"},
			},
			expectedList: []any{"item1", "item2"},
			expectedOk:   true,
		},
		{
			name: "no append tag",
			value: map[string]any{
				"regular": "value",
			},
			expectedList: nil,
			expectedOk:   false,
		},
		{
			name: "append tag with non-list value",
			value: map[string]any{
				"__atmos_append__": "not a list",
			},
			expectedList: nil,
			expectedOk:   false,
		},
		{
			name:         "not a map",
			value:        []any{"item1", "item2"},
			expectedList: nil,
			expectedOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, ok := ExtractAppendListValue(tt.value)
			assert.Equal(t, tt.expectedOk, ok)
			assert.Equal(t, tt.expectedList, list)
		})
	}
}

func TestWrapWithAppendTag(t *testing.T) {
	tests := []struct {
		name     string
		list     []any
		expected map[string]any
	}{
		{
			name: "wrap simple list",
			list: []any{"item1", "item2"},
			expected: map[string]any{
				"__atmos_append__": []any{"item1", "item2"},
			},
		},
		{
			name: "wrap empty list",
			list: []any{},
			expected: map[string]any{
				"__atmos_append__": []any{},
			},
		},
		{
			name: "wrap nil list",
			list: nil,
			expected: map[string]any{
				"__atmos_append__": []any(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapWithAppendTag(tt.list)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessAppendTag(t *testing.T) {
	// Test that ProcessAppendTag doesn't return an error for valid inputs
	err := ProcessAppendTag([]any{"item1", "item2"})
	assert.NoError(t, err)

	err = ProcessAppendTag(map[string]any{"key": "value"})
	assert.NoError(t, err)

	err = ProcessAppendTag(nil)
	assert.NoError(t, err)
}
