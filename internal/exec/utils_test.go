package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPostProcessTemplatesAndYamlFunctions(t *testing.T) {
	tests := []struct {
		name     string
		input    schema.ConfigAndStacksInfo
		expected schema.ConfigAndStacksInfo
	}{
		{
			name: "all-fields-present",
			input: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{
					cfg.ProvidersSectionName:   map[string]any{"aws": map[string]any{"region": "us-west-2"}},
					cfg.AuthSectionName:        map[string]interface{}{"providers": map[string]interface{}{"aws": schema.Provider{Region: "us-west-2"}}},
					cfg.VarsSectionName:        map[string]any{"environment": "dev"},
					cfg.SettingsSectionName:    map[string]any{"enabled": true},
					cfg.EnvSectionName:         map[string]any{"DB_PASSWORD": "secret"},
					cfg.OverridesSectionName:   map[string]any{"cpu": "1024"},
					cfg.MetadataSectionName:    map[string]any{"description": "test component"},
					cfg.BackendSectionName:     map[string]any{"bucket": "my-bucket"},
					cfg.BackendTypeSectionName: "s3",
					cfg.ComponentSectionName:   "vpc",
					cfg.CommandSectionName:     "apply",
					cfg.WorkspaceSectionName:   "dev",
				},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentProvidersSection: map[string]any{"aws": map[string]any{"region": "us-west-2"}},
				ComponentAuthSection:      schema.AuthConfig{Providers: map[string]schema.Provider{"aws": {Region: "us-west-2"}}},
				ComponentVarsSection:      map[string]any{"environment": "dev"},
				ComponentSettingsSection:  map[string]any{"enabled": true},
				ComponentEnvSection:       map[string]any{"DB_PASSWORD": "secret"},
				ComponentOverridesSection: map[string]any{"cpu": "1024"},
				ComponentMetadataSection:  map[string]any{"description": "test component"},
				ComponentBackendSection:   map[string]any{"bucket": "my-bucket"},
				ComponentBackendType:      "s3",
				Component:                 "vpc",
				Command:                   "apply",
				TerraformWorkspace:        "dev",
			},
		},
		{
			name: "partial-fields",
			input: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{
					cfg.VarsSectionName:      map[string]any{"environment": "prod"},
					cfg.SettingsSectionName:  map[string]any{"enabled": false},
					cfg.ComponentSectionName: "eks",
				},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentVarsSection:     map[string]any{"environment": "prod"},
				ComponentSettingsSection: map[string]any{"enabled": false},
				Component:                "eks",
			},
		},
		{
			name: "empty-component-section",
			input: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{},
			},
			expected: schema.ConfigAndStacksInfo{
				ComponentSection: map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy of the input to avoid modifying the test case
			input := tt.input

			// Call the function being tested
			postProcessTemplatesAndYamlFunctions(&input)

			// Compare each expected field individually for better error messages
			assert.Equal(t, tt.expected.ComponentProvidersSection, input.ComponentProvidersSection, "ComponentProvidersSection mismatch")
			assert.Equal(t, tt.expected.ComponentAuthSection, input.ComponentAuthSection, "ComponentAuthSection mismatch")
			assert.Equal(t, tt.expected.ComponentVarsSection, input.ComponentVarsSection, "ComponentVarsSection mismatch")
			assert.Equal(t, tt.expected.ComponentSettingsSection, input.ComponentSettingsSection, "ComponentSettingsSection mismatch")
			assert.Equal(t, tt.expected.ComponentEnvSection, input.ComponentEnvSection, "ComponentEnvSection mismatch")
			assert.Equal(t, tt.expected.ComponentOverridesSection, input.ComponentOverridesSection, "ComponentOverridesSection mismatch")
			assert.Equal(t, tt.expected.ComponentMetadataSection, input.ComponentMetadataSection, "ComponentMetadataSection mismatch")
			assert.Equal(t, tt.expected.ComponentBackendSection, input.ComponentBackendSection, "ComponentBackendSection mismatch")
			assert.Equal(t, tt.expected.ComponentBackendType, input.ComponentBackendType, "ComponentBackendType mismatch")
			assert.Equal(t, tt.expected.Component, input.Component, "Component mismatch")
			assert.Equal(t, tt.expected.Command, input.Command, "Command mismatch")
			assert.Equal(t, tt.expected.TerraformWorkspace, input.TerraformWorkspace, "TerraformWorkspace mismatch")
		})
	}
}
