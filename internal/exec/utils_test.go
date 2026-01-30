package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
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
			result := generateComponentProviderOverrides(tt.providerOverrides, nil)
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
		{
			name:               "empty-backend-type-returns-error",
			backendType:        "",
			backendConfig:      map[string]any{},
			terraformWorkspace: "",
			expected:           nil,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateComponentBackendConfig(tt.backendType, tt.backendConfig, tt.terraformWorkspace, nil)

			if tt.expectError {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrBackendTypeRequired)
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
// TestFilterComponentLocals tests the filterComponentLocals function.
func TestFilterComponentLocals(t *testing.T) {
	tests := []struct {
		name                    string
		originalComponentLocals schema.AtmosSectionMapType
		componentSection        map[string]any
		expectedLocals          map[string]any // nil means locals key should be deleted.
		expectLocalsUnchanged   bool           // When true, locals should remain as-is (e.g. non-map locals).
	}{
		{
			name:                    "no original component locals - deletes merged locals",
			originalComponentLocals: map[string]any{},
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"stack_local": "should_be_removed",
				},
			},
			expectedLocals: nil,
		},
		{
			name:                    "nil original component locals - deletes merged locals",
			originalComponentLocals: nil,
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"stack_local": "should_be_removed",
				},
			},
			expectedLocals: nil,
		},
		{
			name: "filters to keep only component-level locals",
			originalComponentLocals: map[string]any{
				"engine":  true,
				"version": true,
			},
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"engine":      "postgres",
					"version":     "15",
					"stack_local": "should_be_removed",
					"namespace":   "also_removed",
				},
			},
			expectedLocals: map[string]any{
				"engine":  "postgres",
				"version": "15",
			},
		},
		{
			name: "all original keys match - preserves all",
			originalComponentLocals: map[string]any{
				"key1": true,
				"key2": true,
			},
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"key1": "value1",
					"key2": "value2",
				},
			},
			expectedLocals: map[string]any{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "no matching keys - deletes locals",
			originalComponentLocals: map[string]any{
				"missing_key": true,
			},
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"other_key": "value",
				},
			},
			expectedLocals: nil,
		},
		{
			name: "locals not a map - returns early",
			originalComponentLocals: map[string]any{
				"key1": true,
			},
			componentSection: map[string]any{
				cfg.LocalsSectionName: "not_a_map",
			},
			// Non-map locals should remain unchanged.
			expectedLocals:        nil, // special case: not a map, function returns early.
			expectLocalsUnchanged: true,
		},
		{
			name: "no locals key in component section",
			originalComponentLocals: map[string]any{
				"key1": true,
			},
			componentSection: map[string]any{
				"vars": map[string]any{"stage": "dev"},
			},
			expectedLocals: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{
				OriginalComponentLocals: tt.originalComponentLocals,
				ComponentSection:        tt.componentSection,
			}

			filterComponentLocals(info)

			if tt.expectedLocals == nil {
				// Locals should either be deleted or not present.
				_, hasLocals := info.ComponentSection[cfg.LocalsSectionName]
				if tt.expectLocalsUnchanged {
					// Non-map locals: function returns early without modifying.
					assert.True(t, hasLocals, "non-map locals should remain")
				} else {
					assert.False(t, hasLocals, "locals should be deleted from component section")
				}
			} else {
				locals, ok := info.ComponentSection[cfg.LocalsSectionName].(map[string]any)
				assert.True(t, ok, "locals should be a map")
				assert.Equal(t, tt.expectedLocals, locals)
			}
		})
	}
}

// TestPostProcessTemplatesAndYamlFunctions_WithLocalsFiltering tests that postProcessTemplatesAndYamlFunctions
// calls filterComponentLocals correctly.
func TestPostProcessTemplatesAndYamlFunctions_WithLocalsFiltering(t *testing.T) {
	tests := []struct {
		name                    string
		originalComponentLocals schema.AtmosSectionMapType
		componentSection        map[string]any
		expectLocalsPresent     bool
	}{
		{
			name:                    "stack-level locals removed when no original component locals",
			originalComponentLocals: map[string]any{},
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"stack_local": "value",
				},
			},
			expectLocalsPresent: false,
		},
		{
			name: "component-level locals preserved",
			originalComponentLocals: map[string]any{
				"engine": true,
			},
			componentSection: map[string]any{
				cfg.LocalsSectionName: map[string]any{
					"engine":      "postgres",
					"stack_local": "removed",
				},
			},
			expectLocalsPresent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{
				OriginalComponentLocals: tt.originalComponentLocals,
				ComponentSection:        tt.componentSection,
			}

			postProcessTemplatesAndYamlFunctions(info)

			_, hasLocals := info.ComponentSection[cfg.LocalsSectionName]
			assert.Equal(t, tt.expectLocalsPresent, hasLocals)
		})
	}
}

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

// TestProcessComponentConfig_LocalsMerging tests the locals merging logic in ProcessComponentConfig.
// This covers the new code that merges stack-level and component-level locals.
func TestProcessComponentConfig_LocalsMerging(t *testing.T) {
	tests := []struct {
		name                    string
		stacksMap               map[string]any
		stack                   string
		component               string
		originalComponentLocals schema.AtmosSectionMapType
		expectedLocalsKeys      []string
		expectedLocals          map[string]any // When set, assert exact locals values.
	}{
		{
			name: "component with locals and stack locals merged",
			stacksMap: map[string]any{
				"dev": map[string]any{
					cfg.LocalsSectionName: map[string]any{
						"stack_local": "stack_value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
								cfg.LocalsSectionName: map[string]any{
									"component_local": "component_value",
								},
								cfg.MetadataSectionName: map[string]any{},
							},
						},
					},
				},
			},
			stack:              "dev",
			component:          "vpc",
			expectedLocalsKeys: []string{"component_local", "stack_local"},
		},
		{
			name: "component without locals but stack has locals",
			stacksMap: map[string]any{
				"dev": map[string]any{
					cfg.LocalsSectionName: map[string]any{
						"stack_local": "stack_value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
								cfg.MetadataSectionName: map[string]any{},
							},
						},
					},
				},
			},
			stack:              "dev",
			component:          "vpc",
			expectedLocalsKeys: []string{"stack_local"},
		},
		{
			name: "component with locals but no stack locals",
			stacksMap: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
								cfg.LocalsSectionName: map[string]any{
									"component_local": "component_value",
								},
								cfg.MetadataSectionName: map[string]any{},
							},
						},
					},
				},
			},
			stack:              "dev",
			component:          "vpc",
			expectedLocalsKeys: []string{"component_local"},
		},
		{
			name: "component local overrides stack local with same key",
			stacksMap: map[string]any{
				"dev": map[string]any{
					cfg.LocalsSectionName: map[string]any{
						"stage": "stack_dev",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
								cfg.LocalsSectionName: map[string]any{
									"stage": "component_dev",
								},
								cfg.MetadataSectionName: map[string]any{},
							},
						},
					},
				},
			},
			stack:              "dev",
			component:          "vpc",
			expectedLocalsKeys: []string{"stage"},
			expectedLocals:     map[string]any{"stage": "component_dev"},
		},
		{
			name: "preserves OriginalComponentLocals across calls",
			stacksMap: map[string]any{
				"dev": map[string]any{
					cfg.LocalsSectionName: map[string]any{
						"stack_local": "stack_value",
					},
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"vars": map[string]any{
									"region": "us-east-1",
								},
								cfg.LocalsSectionName: map[string]any{
									"component_local": "component_value",
								},
								cfg.MetadataSectionName: map[string]any{},
							},
						},
					},
				},
			},
			stack:     "dev",
			component: "vpc",
			originalComponentLocals: map[string]any{
				"component_local": true,
			},
			expectedLocalsKeys: []string{"component_local", "stack_local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			configAndStacksInfo := schema.ConfigAndStacksInfo{
				ComponentFromArg:        tt.component,
				Stack:                   tt.stack,
				ComponentType:           "terraform",
				OriginalComponentLocals: tt.originalComponentLocals,
			}

			err := ProcessComponentConfig(atmosConfig, &configAndStacksInfo, tt.stack, tt.stacksMap, "terraform", tt.component, nil)
			assert.NoError(t, err)

			// Verify locals are merged in the component section.
			if tt.expectedLocalsKeys != nil {
				localsSection, ok := configAndStacksInfo.ComponentSection[cfg.LocalsSectionName].(map[string]any)
				if len(tt.expectedLocalsKeys) > 0 {
					assert.True(t, ok, "locals should be present in component section")
					assert.Len(t, localsSection, len(tt.expectedLocalsKeys), "locals should have exactly the expected keys")
					for _, key := range tt.expectedLocalsKeys {
						_, exists := localsSection[key]
						assert.True(t, exists, "expected local key %q to be present", key)
					}
					if tt.expectedLocals != nil {
						assert.Equal(t, tt.expectedLocals, localsSection)
					}
				}
			}

			// Verify OriginalComponentLocals is set.
			assert.NotNil(t, configAndStacksInfo.OriginalComponentLocals,
				"OriginalComponentLocals should be initialized after ProcessComponentConfig")
		})
	}
}

// TestProcessComponentConfig_LocalsMergingDoesNotMutateOriginal tests that the locals
// merging in ProcessComponentConfig does not mutate the original stacksMap.
func TestProcessComponentConfig_LocalsMergingDoesNotMutateOriginal(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			cfg.LocalsSectionName: map[string]any{
				"stack_local": "stack_value",
			},
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": map[string]any{
						"vars": map[string]any{
							"region": "us-east-1",
						},
						cfg.LocalsSectionName: map[string]any{
							"component_local": "component_value",
						},
						cfg.MetadataSectionName: map[string]any{},
					},
				},
			},
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: "vpc",
		Stack:            "dev",
		ComponentType:    "terraform",
	}

	err := ProcessComponentConfig(atmosConfig, &configAndStacksInfo, "dev", stacksMap, "terraform", "vpc", nil)
	assert.NoError(t, err)

	// Verify original stacksMap was not mutated.
	originalComponent := stacksMap["dev"].(map[string]any)["components"].(map[string]any)["terraform"].(map[string]any)["vpc"].(map[string]any)
	originalLocals := originalComponent[cfg.LocalsSectionName].(map[string]any)

	// The original should NOT have the stack_local merged in.
	_, hasStackLocal := originalLocals["stack_local"]
	assert.False(t, hasStackLocal, "original stacksMap should not be mutated with stack-level locals")
	assert.Equal(t, "component_value", originalLocals["component_local"])
}
