package exec

import (
	"os"
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

func TestTFCliArgsAndVarsComponentSections(t *testing.T) {
	tests := []struct {
		name                string
		tfCliArgsEnv        string
		expectedHasArgs     bool
		expectedHasVars     bool
		expectedArgsCount   int
		expectedVarsCount   int
		expectedSpecificVar string
		expectedSpecificVal any
	}{
		{
			name:              "empty TF_CLI_ARGS",
			tfCliArgsEnv:      "",
			expectedHasArgs:   false,
			expectedHasVars:   false,
			expectedArgsCount: 0,
			expectedVarsCount: 0,
		},
		{
			name:              "TF_CLI_ARGS with arguments only",
			tfCliArgsEnv:      "-auto-approve -input=false",
			expectedHasArgs:   true,
			expectedHasVars:   false,
			expectedArgsCount: 2,
			expectedVarsCount: 0,
		},
		{
			name:                "TF_CLI_ARGS with variables only",
			tfCliArgsEnv:        "-var environment=test -var region=us-west-2",
			expectedHasArgs:     true,
			expectedHasVars:     true,
			expectedArgsCount:   4,
			expectedVarsCount:   2,
			expectedSpecificVar: "environment",
			expectedSpecificVal: "test",
		},
		{
			name:                "TF_CLI_ARGS with mixed args and vars",
			tfCliArgsEnv:        "-auto-approve -var environment=prod -var count=5 -input=false",
			expectedHasArgs:     true,
			expectedHasVars:     true,
			expectedArgsCount:   6,
			expectedVarsCount:   2,
			expectedSpecificVar: "count",
			expectedSpecificVal: float64(5), // JSON numbers become float64
		},
		{
			name:                "TF_CLI_ARGS with JSON variable",
			tfCliArgsEnv:        `-var 'tags={"env":"prod","team":"devops"}'`,
			expectedHasArgs:     true,
			expectedHasVars:     true,
			expectedArgsCount:   2,
			expectedVarsCount:   1,
			expectedSpecificVar: "tags",
			expectedSpecificVal: map[string]any{"env": "prod", "team": "devops"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original value to restore later
			originalValue := os.Getenv("TF_CLI_ARGS")
			defer func() {
				if originalValue != "" {
					os.Setenv("TF_CLI_ARGS", originalValue)
				} else {
					os.Unsetenv("TF_CLI_ARGS")
				}
			}()

			// Set test environment variable
			if tt.tfCliArgsEnv != "" {
				os.Setenv("TF_CLI_ARGS", tt.tfCliArgsEnv)
			} else {
				os.Unsetenv("TF_CLI_ARGS")
			}

			// Create a component section to simulate what ProcessStacks does
			componentSection := make(map[string]any)

			// Test the TF_CLI_ARGS functionality directly
			tfEnvCliArgs := GetTerraformEnvCliArgs()
			if len(tfEnvCliArgs) > 0 {
				componentSection[cfg.TerraformCliArgsEnvSectionName] = tfEnvCliArgs
			}

			tfEnvCliVars, err := GetTerraformEnvCliVars()
			assert.NoError(t, err, "GetTerraformEnvCliVars should not return an error")
			if len(tfEnvCliVars) > 0 {
				componentSection[cfg.TerraformCliVarsEnvSectionName] = tfEnvCliVars
			}

			// Check env_tf_cli_args section
			if tt.expectedHasArgs {
				args, exists := componentSection[cfg.TerraformCliArgsEnvSectionName]
				assert.True(t, exists, "env_tf_cli_args section should exist")
				argsSlice, ok := args.([]string)
				assert.True(t, ok, "env_tf_cli_args should be a slice of strings")
				assert.Len(t, argsSlice, tt.expectedArgsCount, "env_tf_cli_args should have expected number of arguments")
			} else {
				_, exists := componentSection[cfg.TerraformCliArgsEnvSectionName]
				assert.False(t, exists, "env_tf_cli_args section should not exist when no arguments")
			}

			// Check env_tf_cli_vars section
			if tt.expectedHasVars {
				vars, exists := componentSection[cfg.TerraformCliVarsEnvSectionName]
				assert.True(t, exists, "env_tf_cli_vars section should exist")
				varsMap, ok := vars.(map[string]any)
				assert.True(t, ok, "env_tf_cli_vars should be a map")
				assert.Len(t, varsMap, tt.expectedVarsCount, "env_tf_cli_vars should have expected number of variables")

				// Check specific variable if provided
				if tt.expectedSpecificVar != "" {
					value, exists := varsMap[tt.expectedSpecificVar]
					assert.True(t, exists, "specific variable should exist")
					assert.Equal(t, tt.expectedSpecificVal, value, "specific variable should have expected value")
				}
			} else {
				_, exists := componentSection[cfg.TerraformCliVarsEnvSectionName]
				assert.False(t, exists, "env_tf_cli_vars section should not exist when no variables")
			}
		})
	}
}
