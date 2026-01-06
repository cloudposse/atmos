package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDeriveStackFileName(t *testing.T) {
	tests := []struct {
		name           string
		stacksBasePath string
		filePath       string
		expected       string
	}{
		{
			name:           "simple file path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/dev.yaml",
			expected:       "dev",
		},
		{
			name:           "nested file path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/deploy/dev.yaml",
			expected:       "deploy/dev",
		},
		{
			name:           "deeply nested file path",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/org/team/deploy/dev.yaml",
			expected:       "org/team/deploy/dev",
		},
		{
			name:           "empty base path falls back to filename",
			stacksBasePath: "",
			filePath:       "/path/to/stacks/deploy/dev.yaml",
			expected:       "dev",
		},
		{
			name:           "yml extension",
			stacksBasePath: "/path/to/stacks",
			filePath:       "/path/to/stacks/prod.yml",
			expected:       "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &mockAtmosConfig{
				stacksBaseAbsolutePath: tt.stacksBasePath,
			}

			result := deriveStackFileName(atmosConfig.toSchema(), tt.filePath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeriveStackName(t *testing.T) {
	tests := []struct {
		name            string
		stackFileName   string
		varsSection     map[string]any
		stackSectionMap map[string]any
		expected        string
	}{
		{
			name:          "explicit name in manifest",
			stackFileName: "deploy/dev",
			varsSection:   nil,
			stackSectionMap: map[string]any{
				"name": "my-custom-stack-name",
			},
			expected: "my-custom-stack-name",
		},
		{
			name:          "empty name falls back to filename",
			stackFileName: "deploy/dev",
			varsSection:   nil,
			stackSectionMap: map[string]any{
				"name": "",
			},
			expected: "deploy/dev",
		},
		{
			name:            "no name uses filename",
			stackFileName:   "deploy/prod",
			varsSection:     nil,
			stackSectionMap: map[string]any{},
			expected:        "deploy/prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &mockAtmosConfig{}

			result := deriveStackName(atmosConfig.toSchema(), tt.stackFileName, tt.varsSection, tt.stackSectionMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockAtmosConfig is a helper for creating test configurations.
type mockAtmosConfig struct {
	stacksBaseAbsolutePath string
	nameTemplate           string
	namePattern            string
}

func (m *mockAtmosConfig) toSchema() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		StacksBaseAbsolutePath: m.stacksBaseAbsolutePath,
		Stacks: schema.Stacks{
			NameTemplate: m.nameTemplate,
			NamePattern:  m.namePattern,
		},
	}
}
