package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendJSONPathKey(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		key      string
		expected string
	}{
		{
			name:     "append to non-empty path",
			basePath: "vars",
			key:      "name",
			expected: "vars.name",
		},
		{
			name:     "append to empty path",
			basePath: "",
			key:      "vars",
			expected: "vars",
		},
		{
			name:     "append empty key",
			basePath: "vars",
			key:      "",
			expected: "vars",
		},
		{
			name:     "both empty",
			basePath: "",
			key:      "",
			expected: "",
		},
		{
			name:     "nested path",
			basePath: "vars.tags",
			key:      "environment",
			expected: "vars.tags.environment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendJSONPathKey(tt.basePath, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAppendJSONPathIndex(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		index    int
		expected string
	}{
		{
			name:     "append to non-empty path",
			basePath: "vars.zones",
			index:    0,
			expected: "vars.zones[0]",
		},
		{
			name:     "append to empty path",
			basePath: "",
			index:    0,
			expected: "[0]",
		},
		{
			name:     "large index",
			basePath: "vars.zones",
			index:    42,
			expected: "vars.zones[42]",
		},
		{
			name:     "zero index",
			basePath: "items",
			index:    0,
			expected: "items[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AppendJSONPathIndex(tt.basePath, tt.index)
			assert.Equal(t, tt.expected, result)
		})
	}
}
