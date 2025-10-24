package devcontainer

import (
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/stretchr/testify/assert"
)

func TestValidateNotImported(t *testing.T) {
	tests := []struct {
		name        string
		importPath  string
		expectError bool
		errorType   error
	}{
		{
			name:        "valid import path without devcontainer",
			importPath:  "components/terraform/vpc",
			expectError: false,
		},
		{
			name:        "valid import path with common words",
			importPath:  "stacks/dev/container-apps",
			expectError: false,
		},
		{
			name:        "devcontainer.json file",
			importPath:  "devcontainer.json",
			expectError: true,
			errorType:   errUtils.ErrInvalidDevcontainerConfig,
		},
		{
			name:        ".devcontainer/devcontainer.json file",
			importPath:  ".devcontainer/devcontainer.json",
			expectError: true,
			errorType:   errUtils.ErrInvalidDevcontainerConfig,
		},
		{
			name:        ".devcontainer.json file",
			importPath:  ".devcontainer.json",
			expectError: true,
			errorType:   errUtils.ErrInvalidDevcontainerConfig,
		},
		{
			name:        "path ending with devcontainer.json",
			importPath:  "some/path/devcontainer.json",
			expectError: true,
			errorType:   errUtils.ErrInvalidDevcontainerConfig,
		},
		{
			name:        "path ending with .devcontainer.json",
			importPath:  "my/config/.devcontainer.json",
			expectError: true,
			errorType:   errUtils.ErrInvalidDevcontainerConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotImported(tt.importPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.errorType)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestContainsDevcontainerConfig(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "exact match devcontainer.json",
			path:     "devcontainer.json",
			expected: true,
		},
		{
			name:     "exact match .devcontainer/devcontainer.json",
			path:     ".devcontainer/devcontainer.json",
			expected: true,
		},
		{
			name:     "exact match .devcontainer.json",
			path:     ".devcontainer.json",
			expected: true,
		},
		{
			name:     "path ending with devcontainer.json",
			path:     "config/devcontainer.json",
			expected: true,
		},
		{
			name:     "path ending with .devcontainer.json",
			path:     "my/.devcontainer.json",
			expected: true,
		},
		{
			name:     "path ending with .devcontainer/devcontainer.json",
			path:     "some/path/.devcontainer/devcontainer.json",
			expected: true,
		},
		{
			name:     "no devcontainer config",
			path:     "components/terraform/vpc",
			expected: false,
		},
		{
			name:     "contains word devcontainer but not config",
			path:     "stacks/devcontainer-test",
			expected: false,
		},
		{
			name:     "empty path",
			path:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsDevcontainerConfig(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{
			name:     "substring at end",
			s:        "hello world",
			substr:   "world",
			expected: true,
		},
		{
			name:     "substring not at end",
			s:        "hello world",
			substr:   "hello",
			expected: false,
		},
		{
			name:     "exact match",
			s:        "test",
			substr:   "test",
			expected: true,
		},
		{
			name:     "substring longer than string",
			s:        "hi",
			substr:   "hello",
			expected: false,
		},
		{
			name:     "empty substring",
			s:        "test",
			substr:   "",
			expected: true,
		},
		{
			name:     "empty string and substring",
			s:        "",
			substr:   "",
			expected: true,
		},
		{
			name:     "substring in middle",
			s:        "path/to/file.json",
			substr:   "file.json",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			assert.Equal(t, tt.expected, result)
		})
	}
}
