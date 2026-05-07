package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComponentDependency_IsFileDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      ComponentDependency
		expected bool
	}{
		{
			name:     "file kind returns true",
			dep:      ComponentDependency{Kind: "file", Path: "config.json"},
			expected: true,
		},
		{
			name:     "file kind without path still returns true",
			dep:      ComponentDependency{Kind: "file"},
			expected: true,
		},
		{
			name:     "folder kind returns false",
			dep:      ComponentDependency{Kind: "folder", Path: "src/"},
			expected: false,
		},
		{
			name:     "terraform kind returns false",
			dep:      ComponentDependency{Kind: "terraform", Component: "vpc"},
			expected: false,
		},
		{
			name:     "empty kind returns false",
			dep:      ComponentDependency{Component: "vpc"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dep.IsFileDependency())
		})
	}
}

func TestComponentDependency_IsFolderDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      ComponentDependency
		expected bool
	}{
		{
			name:     "folder kind returns true",
			dep:      ComponentDependency{Kind: "folder", Path: "src/lambda"},
			expected: true,
		},
		{
			name:     "folder kind without path still returns true",
			dep:      ComponentDependency{Kind: "folder"},
			expected: true,
		},
		{
			name:     "file kind returns false",
			dep:      ComponentDependency{Kind: "file", Path: "config.json"},
			expected: false,
		},
		{
			name:     "terraform kind returns false",
			dep:      ComponentDependency{Kind: "terraform", Component: "vpc"},
			expected: false,
		},
		{
			name:     "empty kind returns false",
			dep:      ComponentDependency{Component: "rds"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dep.IsFolderDependency())
		})
	}
}

func TestComponentDependency_IsComponentDependency(t *testing.T) {
	tests := []struct {
		name     string
		dep      ComponentDependency
		expected bool
	}{
		{
			name:     "terraform kind returns true",
			dep:      ComponentDependency{Kind: "terraform", Component: "vpc"},
			expected: true,
		},
		{
			name:     "helmfile kind returns true",
			dep:      ComponentDependency{Kind: "helmfile", Component: "nginx"},
			expected: true,
		},
		{
			name:     "empty kind returns true",
			dep:      ComponentDependency{Component: "vpc"},
			expected: true,
		},
		{
			name:     "packer kind returns true",
			dep:      ComponentDependency{Kind: "packer", Component: "ami"},
			expected: true,
		},
		{
			name:     "plugin kind returns true",
			dep:      ComponentDependency{Kind: "plugin", Component: "custom"},
			expected: true,
		},
		{
			name:     "file kind returns false",
			dep:      ComponentDependency{Kind: "file", Path: "config.json"},
			expected: false,
		},
		{
			name:     "folder kind returns false",
			dep:      ComponentDependency{Kind: "folder", Path: "src/"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.dep.IsComponentDependency())
		})
	}
}
