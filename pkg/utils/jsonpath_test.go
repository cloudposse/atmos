package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildJSONPath(t *testing.T) {
	tests := []struct {
		name       string
		components []string
		expected   string
	}{
		{
			name:       "empty components",
			components: []string{},
			expected:   "",
		},
		{
			name:       "single component",
			components: []string{"vars"},
			expected:   "vars",
		},
		{
			name:       "two components",
			components: []string{"vars", "name"},
			expected:   "vars.name",
		},
		{
			name:       "nested components",
			components: []string{"vars", "tags", "environment"},
			expected:   "vars.tags.environment",
		},
		{
			name:       "with empty components",
			components: []string{"vars", "", "name"},
			expected:   "vars.name",
		},
		{
			name:       "all empty components",
			components: []string{"", "", ""},
			expected:   "",
		},
		{
			name:       "with array index",
			components: []string{"import", "[0]"},
			expected:   "import[0]",
		},
		{
			name:       "root array index",
			components: []string{"[0]"},
			expected:   "[0]",
		},
		{
			name:       "multiple array indices",
			components: []string{"matrix", "[0]", "[1]"},
			expected:   "matrix[0][1]",
		},
		{
			name:       "mixed keys and indices",
			components: []string{"vars", "zones", "[0]", "name"},
			expected:   "vars.zones[0].name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildJSONPath(tt.components...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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

func TestSplitJSONPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: []string{},
		},
		{
			name:     "single component",
			path:     "vars",
			expected: []string{"vars"},
		},
		{
			name:     "two components",
			path:     "vars.name",
			expected: []string{"vars", "name"},
		},
		{
			name:     "nested components",
			path:     "vars.tags.environment",
			expected: []string{"vars", "tags", "environment"},
		},
		{
			name:     "with array index",
			path:     "vars.zones[0]",
			expected: []string{"vars", "zones", "[0]"},
		},
		{
			name:     "multiple array indices",
			path:     "matrix[0][1]",
			expected: []string{"matrix", "[0]", "[1]"},
		},
		{
			name:     "array index only",
			path:     "[0]",
			expected: []string{"[0]"},
		},
		{
			name:     "complex path",
			path:     "vars.availability_zones[0].name",
			expected: []string{"vars", "availability_zones", "[0]", "name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitJSONPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseJSONPathIndex(t *testing.T) {
	tests := []struct {
		name          string
		component     string
		expectedIndex int
		expectedOk    bool
	}{
		{
			name:          "valid index",
			component:     "[0]",
			expectedIndex: 0,
			expectedOk:    true,
		},
		{
			name:          "large index",
			component:     "[42]",
			expectedIndex: 42,
			expectedOk:    true,
		},
		{
			name:          "not an index",
			component:     "name",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "invalid format - no closing bracket",
			component:     "[0",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "invalid format - no opening bracket",
			component:     "0]",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "invalid format - non-numeric",
			component:     "[abc]",
			expectedIndex: 0,
			expectedOk:    false,
		},
		{
			name:          "empty brackets",
			component:     "[]",
			expectedIndex: 0,
			expectedOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			index, ok := ParseJSONPathIndex(tt.component)
			assert.Equal(t, tt.expectedOk, ok)
			if tt.expectedOk {
				assert.Equal(t, tt.expectedIndex, index)
			}
		})
	}
}

func TestIsJSONPathIndex(t *testing.T) {
	tests := []struct {
		name      string
		component string
		expected  bool
	}{
		{
			name:      "valid index",
			component: "[0]",
			expected:  true,
		},
		{
			name:      "large index",
			component: "[42]",
			expected:  true,
		},
		{
			name:      "not an index",
			component: "name",
			expected:  false,
		},
		{
			name:      "invalid format",
			component: "[abc]",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsJSONPathIndex(tt.component)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetJSONPathParent(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "root level",
			path:     "vars",
			expected: "",
		},
		{
			name:     "two levels",
			path:     "vars.name",
			expected: "vars",
		},
		{
			name:     "three levels",
			path:     "vars.tags.environment",
			expected: "vars.tags",
		},
		{
			name:     "with array index",
			path:     "vars.zones[0]",
			expected: "vars.zones",
		},
		{
			name:     "array index only",
			path:     "[0]",
			expected: "",
		},
		{
			name:     "nested array indices",
			path:     "matrix[0][1]",
			expected: "matrix[0]",
		},
		{
			name:     "complex path",
			path:     "vars.availability_zones[0].name",
			expected: "vars.availability_zones[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetJSONPathParent(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetJSONPathLeaf(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "single component",
			path:     "vars",
			expected: "vars",
		},
		{
			name:     "two components",
			path:     "vars.name",
			expected: "name",
		},
		{
			name:     "three components",
			path:     "vars.tags.environment",
			expected: "environment",
		},
		{
			name:     "with array index",
			path:     "vars.zones[0]",
			expected: "[0]",
		},
		{
			name:     "array index only",
			path:     "[0]",
			expected: "[0]",
		},
		{
			name:     "nested array indices",
			path:     "matrix[0][1]",
			expected: "[1]",
		},
		{
			name:     "complex path",
			path:     "vars.availability_zones[0].name",
			expected: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetJSONPathLeaf(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestJSONPath_RoundTrip(t *testing.T) {
	// Test that building and splitting are inverse operations.
	tests := []struct {
		name       string
		components []string
	}{
		{
			name:       "simple path",
			components: []string{"vars", "name"},
		},
		{
			name:       "nested path",
			components: []string{"vars", "tags", "environment"},
		},
		{
			name:       "single component",
			components: []string{"vars"},
		},
		{
			name:       "with array index",
			components: []string{"import", "[0]"},
		},
		{
			name:       "root array index",
			components: []string{"[0]"},
		},
		{
			name:       "multiple array indices",
			components: []string{"matrix", "[0]", "[1]"},
		},
		{
			name:       "mixed keys and indices",
			components: []string{"vars", "zones", "[0]", "name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := BuildJSONPath(tt.components...)
			split := SplitJSONPath(path)
			assert.Equal(t, tt.components, split)
		})
	}
}

func TestJSONPath_ParentChildRelationship(t *testing.T) {
	// Test that parent + leaf = original path.
	tests := []struct {
		name string
		path string
	}{
		{
			name: "simple path",
			path: "vars.name",
		},
		{
			name: "nested path",
			path: "vars.tags.environment",
		},
		{
			name: "with array",
			path: "vars.zones[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := GetJSONPathParent(tt.path)
			leaf := GetJSONPathLeaf(tt.path)

			// Reconstruct the path from parent and leaf.
			var reconstructed string
			switch {
			case parent == "":
				reconstructed = leaf
			case IsJSONPathIndex(leaf):
				reconstructed = parent + leaf
			default:
				reconstructed = AppendJSONPathKey(parent, leaf)
			}

			assert.Equal(t, tt.path, reconstructed)
		})
	}
}
