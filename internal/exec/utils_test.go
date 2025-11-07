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
					cfg.AuthSectionName:        map[string]interface{}{"providers": map[string]schema.Provider{"aws": {Region: "us-west-2"}}},
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
				ComponentAuthSection:      schema.AtmosSectionMapType{"providers": map[string]schema.Provider{"aws": {Region: "us-west-2"}}},
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

func TestGenerateComponentProviderOverrides(t *testing.T) {
	tests := []struct {
		name              string
		providerOverrides map[string]any
		expected          map[string]any
	}{
		{
			name: "single-provider",
			providerOverrides: map[string]any{
				"aws": map[string]any{
					"region": "us-west-2",
					"alias":  "west",
				},
			},
			expected: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
						"alias":  "west",
					},
				},
			},
		},
		{
			name: "multiple-providers",
			providerOverrides: map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
				"google": map[string]any{
					"project": "my-project",
					"region":  "us-central1",
				},
			},
			expected: map[string]any{
				"provider": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
					"google": map[string]any{
						"project": "my-project",
						"region":  "us-central1",
					},
				},
			},
		},
		{
			name:              "empty-overrides",
			providerOverrides: map[string]any{},
			expected: map[string]any{
				"provider": map[string]any{},
			},
		},
		{
			name:              "nil-overrides",
			providerOverrides: nil,
			expected: map[string]any{
				"provider": map[string]any(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateComponentProviderOverrides(tt.providerOverrides)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateComponentBackendConfig(t *testing.T) {
	tests := []struct {
		name               string
		backendType        string
		backendConfig      map[string]any
		terraformWorkspace string
		expected           map[string]any
		expectError        bool
	}{
		{
			name:        "s3-backend",
			backendType: "s3",
			backendConfig: map[string]any{
				"bucket":  "my-bucket",
				"key":     "terraform.tfstate",
				"region":  "us-west-2",
				"encrypt": true,
			},
			terraformWorkspace: "dev",
			expected: map[string]any{
				"terraform": map[string]any{
					"backend": map[string]any{
						"s3": map[string]any{
							"bucket":  "my-bucket",
							"key":     "terraform.tfstate",
							"region":  "us-west-2",
							"encrypt": true,
						},
					},
				},
			},
		},
		{
			name:        "cloud-backend-with-workspace",
			backendType: "cloud",
			backendConfig: map[string]any{
				"organization": "my-org",
				"workspaces": map[string]any{
					"name": "{terraform_workspace}-app",
				},
			},
			terraformWorkspace: "staging",
			expected: map[string]any{
				"terraform": map[string]any{
					"cloud": map[string]any{
						"organization": "my-org",
						"workspaces": map[string]any{
							"name": "staging-app",
						},
					},
				},
			},
		},
		{
			name:        "cloud-backend-without-workspace",
			backendType: "cloud",
			backendConfig: map[string]any{
				"organization": "my-org",
				"workspaces": map[string]any{
					"name": "my-workspace",
				},
			},
			terraformWorkspace: "",
			expected: map[string]any{
				"terraform": map[string]any{
					"cloud": map[string]any{
						"organization": "my-org",
						"workspaces": map[string]any{
							"name": "my-workspace",
						},
					},
				},
			},
		},
		{
			name:               "local-backend",
			backendType:        "local",
			backendConfig:      map[string]any{"path": "terraform.tfstate"},
			terraformWorkspace: "dev",
			expected: map[string]any{
				"terraform": map[string]any{
					"backend": map[string]any{
						"local": map[string]any{"path": "terraform.tfstate"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateComponentBackendConfig(tt.backendType, tt.backendConfig, tt.terraformWorkspace, nil)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFindComponentDependencies(t *testing.T) {
	tests := []struct {
		name         string
		currentStack string
		sources      schema.ConfigSources
		expectedDeps []string
		expectedAll  []string
	}{
		{
			name:         "single-dependency",
			currentStack: "stack1.yaml",
			sources: schema.ConfigSources{
				"vars": map[string]schema.ConfigSourcesItem{
					"key1": {
						StackDependencies: schema.ConfigSourcesStackDependencies{
							{StackFile: "base.yaml"},
						},
					},
				},
			},
			expectedDeps: []string{"base.yaml"},
			expectedAll:  []string{"base.yaml", "stack1.yaml"},
		},
		{
			name:         "multiple-dependencies",
			currentStack: "prod.yaml",
			sources: schema.ConfigSources{
				"vars": map[string]schema.ConfigSourcesItem{
					"key1": {
						StackDependencies: schema.ConfigSourcesStackDependencies{
							{StackFile: "base.yaml"},
							{StackFile: "network.yaml"},
						},
					},
					"key2": {
						StackDependencies: schema.ConfigSourcesStackDependencies{
							{StackFile: "security.yaml"},
						},
					},
				},
			},
			expectedDeps: []string{"base.yaml", "security.yaml"},
			expectedAll:  []string{"base.yaml", "network.yaml", "prod.yaml", "security.yaml"},
		},
		{
			name:         "no-dependencies",
			currentStack: "standalone.yaml",
			sources:      schema.ConfigSources{},
			expectedDeps: []string{},
			expectedAll:  []string{"standalone.yaml"},
		},
		{
			name:         "empty-stack-file-ignored",
			currentStack: "test.yaml",
			sources: schema.ConfigSources{
				"vars": map[string]schema.ConfigSourcesItem{
					"key1": {
						StackDependencies: schema.ConfigSourcesStackDependencies{
							{StackFile: ""},
							{StackFile: "valid.yaml"},
						},
					},
				},
			},
			expectedDeps: []string{},
			expectedAll:  []string{"test.yaml", "valid.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, depsAll, err := FindComponentDependencies(tt.currentStack, tt.sources)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedDeps, deps)
			assert.Equal(t, tt.expectedAll, depsAll)
		})
	}
}

// TestGetFindStacksMapCacheKey tests cache key generation for FindStacksMap.
// This validates P3.4 optimization: cache key includes atmosConfig and parameters.
func TestGetFindStacksMapCacheKey(t *testing.T) {
	// Create a test atmosConfig with key fields that are actually used in cache key generation.
	// The cache key uses: StacksBaseAbsolutePath, TerraformDirAbsolutePath, ignoreMissingFiles, len(StackConfigFilesAbsolutePaths)
	atmosConfig1 := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:        "/path/to/stacks",
		TerraformDirAbsolutePath:      "/path/to/components/terraform",
		StackConfigFilesAbsolutePaths: []string{"/path/to/stacks/file1.yaml", "/path/to/stacks/file2.yaml"},
	}

	atmosConfig2 := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:        "/different/path/to/stacks",
		TerraformDirAbsolutePath:      "/path/to/components/terraform",
		StackConfigFilesAbsolutePaths: []string{"/path/to/stacks/file1.yaml", "/path/to/stacks/file2.yaml"},
	}

	atmosConfig3 := &schema.AtmosConfiguration{
		StacksBaseAbsolutePath:        "/path/to/stacks",
		TerraformDirAbsolutePath:      "/path/to/components/terraform",
		StackConfigFilesAbsolutePaths: []string{"/path/to/stacks/file1.yaml"},
	}

	// Test: same config, same parameters → same cache key.
	key1 := getFindStacksMapCacheKey(atmosConfig1, false)
	key2 := getFindStacksMapCacheKey(atmosConfig1, false)
	assert.Equal(t, key1, key2, "Same config and parameters should produce same cache key")

	// Test: same config, different parameters → different cache key.
	key3 := getFindStacksMapCacheKey(atmosConfig1, true)
	assert.NotEqual(t, key1, key3, "Different ignoreMissingFiles should produce different cache key")

	// Test: different StacksBaseAbsolutePath → different cache key.
	key4 := getFindStacksMapCacheKey(atmosConfig2, false)
	assert.NotEqual(t, key1, key4, "Different StacksBaseAbsolutePath should produce different cache key")

	// Test: different StackConfigFilesAbsolutePaths length → different cache key.
	key5 := getFindStacksMapCacheKey(atmosConfig3, false)
	assert.NotEqual(t, key1, key5, "Different StackConfigFilesAbsolutePaths length should produce different cache key")

	// Test: cache keys are not empty.
	assert.NotEmpty(t, key1)
	assert.NotEmpty(t, key3)
	assert.NotEmpty(t, key4)
	assert.NotEmpty(t, key5)
}
