package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessComponent(t *testing.T) {
	tests := []struct {
		name              string
		opts              ComponentProcessorOptions
		expectedError     string
		expectedVars      map[string]any
		expectedSettings  map[string]any
		expectedEnv       map[string]any
		expectedMetadata  map[string]any
		expectedCommand   string
		expectedProviders map[string]any
		expectedHooks     map[string]any
		expectedAuth      map[string]any
	}{
		{
			name: "terraform component with all sections",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"region": "us-east-1",
					},
					cfg.SettingsSectionName: map[string]any{
						"enabled": true,
					},
					cfg.EnvSectionName: map[string]any{
						"ENV": "dev",
					},
					cfg.ProvidersSectionName: map[string]any{
						"aws": map[string]any{
							"region": "us-east-1",
						},
					},
					cfg.HooksSectionName: map[string]any{
						"before": []string{"echo test"},
					},
					cfg.AuthSectionName: map[string]any{
						"oidc": map[string]any{
							"enabled": true,
						},
					},
					cfg.MetadataSectionName: map[string]any{
						"type": "real",
					},
					cfg.CommandSectionName:     "tofu",
					cfg.BackendTypeSectionName: "s3",
					cfg.BackendSectionName: map[string]any{
						"s3": map[string]any{
							"bucket": "test-bucket",
						},
					},
					cfg.RemoteStateBackendTypeSectionName: "s3",
					cfg.RemoteStateBackendSectionName: map[string]any{
						"s3": map[string]any{
							"bucket": "test-state-bucket",
						},
					},
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"region": "us-east-1",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectedEnv: map[string]any{
				"ENV": "dev",
			},
			expectedProviders: map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
			expectedHooks: map[string]any{
				"before": []string{"echo test"},
			},
			expectedAuth: map[string]any{
				"oidc": map[string]any{
					"enabled": true,
				},
			},
			expectedMetadata: map[string]any{
				"type": "real",
			},
			expectedCommand: "tofu",
		},
		{
			name: "helmfile component without terraform-specific sections",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.HelmfileComponentType,
				Component:     "app",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"namespace": "default",
					},
					cfg.SettingsSectionName: map[string]any{
						"enabled": true,
					},
					cfg.EnvSectionName: map[string]any{
						"KUBECONFIG": "/path/to/config",
					},
					cfg.MetadataSectionName: map[string]any{
						"type": "real",
					},
					cfg.CommandSectionName: "helmfile",
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"namespace": "default",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectedEnv: map[string]any{
				"KUBECONFIG": "/path/to/config",
			},
			expectedMetadata: map[string]any{
				"type": "real",
			},
			expectedCommand: "helmfile",
		},
		{
			name: "packer component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.PackerComponentType,
				Component:     "ami",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"ami_name": "test-ami",
					},
					cfg.SettingsSectionName: map[string]any{
						"enabled": true,
					},
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"ami_name": "test-ami",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
		},
		{
			name: "component with overrides",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"region": "us-east-1",
					},
					cfg.OverridesSectionName: map[string]any{
						cfg.VarsSectionName: map[string]any{
							"region": "us-west-2",
						},
						cfg.SettingsSectionName: map[string]any{
							"override": true,
						},
						cfg.CommandSectionName: "tofu",
					},
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"region": "us-east-1",
			},
		},
		{
			name: "component with inheritance",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.ComponentSectionName: "base-vpc",
					cfg.VarsSectionName: map[string]any{
						"region": "us-east-1",
					},
				},
				AllComponentsMap: map[string]any{
					"base-vpc": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
				},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"region": "us-east-1",
			},
		},
		{
			name: "invalid vars section",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: "invalid-not-a-map",
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component vars section",
		},
		{
			name: "invalid settings section",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.SettingsSectionName: []string{"invalid"},
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component settings section",
		},
		{
			name: "terraform component with invalid spacelift settings",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.SettingsSectionName: map[string]any{
						"spacelift": "invalid-not-a-map",
					},
				},
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedError: "invalid spacelift settings section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processComponent(&tt.opts)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectedVars != nil {
				assert.Equal(t, tt.expectedVars, result.ComponentVars)
			}

			if tt.expectedSettings != nil {
				assert.Equal(t, tt.expectedSettings, result.ComponentSettings)
			}

			if tt.expectedEnv != nil {
				assert.Equal(t, tt.expectedEnv, result.ComponentEnv)
			}

			if tt.expectedMetadata != nil {
				assert.Equal(t, tt.expectedMetadata, result.ComponentMetadata)
			}

			if tt.expectedCommand != "" {
				assert.Equal(t, tt.expectedCommand, result.ComponentCommand)
			}

			if tt.expectedProviders != nil {
				assert.Equal(t, tt.expectedProviders, result.ComponentProviders)
			}

			if tt.expectedHooks != nil {
				assert.Equal(t, tt.expectedHooks, result.ComponentHooks)
			}

			if tt.expectedAuth != nil {
				assert.Equal(t, tt.expectedAuth, result.ComponentAuth)
			}
		})
	}
}

func TestExtractComponentSections(t *testing.T) {
	tests := []struct {
		name              string
		opts              ComponentProcessorOptions
		expectedError     string
		expectedVars      map[string]any
		expectedSettings  map[string]any
		expectedEnv       map[string]any
		expectedAuth      map[string]any
		expectedProviders map[string]any
		expectedHooks     map[string]any
		expectedBackend   map[string]any
	}{
		{
			name: "extract all sections for terraform component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"region": "us-east-1",
					},
					cfg.SettingsSectionName: map[string]any{
						"enabled": true,
					},
					cfg.EnvSectionName: map[string]any{
						"AWS_REGION": "us-east-1",
					},
					cfg.AuthSectionName: map[string]any{
						"oidc": map[string]any{
							"enabled": true,
						},
					},
					cfg.ProvidersSectionName: map[string]any{
						"aws": map[string]any{
							"region": "us-east-1",
						},
					},
					cfg.HooksSectionName: map[string]any{
						"before": []any{"echo before"},
					},
					cfg.BackendSectionName: map[string]any{
						"bucket": "test-bucket",
					},
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"region": "us-east-1",
			},
			expectedSettings: map[string]any{
				"enabled": true,
			},
			expectedEnv: map[string]any{
				"AWS_REGION": "us-east-1",
			},
			expectedAuth: map[string]any{
				"oidc": map[string]any{
					"enabled": true,
				},
			},
			expectedProviders: map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
			expectedHooks: map[string]any{
				"before": []any{"echo before"},
			},
			expectedBackend: map[string]any{
				"bucket": "test-bucket",
			},
		},
		{
			name: "extract sections for helmfile component without terraform-specific sections",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.HelmfileComponentType,
				Component:     "app",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"namespace": "default",
					},
					cfg.AuthSectionName: map[string]any{
						"oidc": map[string]any{
							"enabled": true,
						},
					},
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedVars: map[string]any{
				"namespace": "default",
			},
			expectedAuth: map[string]any{
				"oidc": map[string]any{
					"enabled": true,
				},
			},
		},
		{
			name: "invalid vars section type",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.VarsSectionName: "invalid-string",
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component vars section",
		},
		{
			name: "invalid auth section type",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.AuthSectionName: "invalid-string",
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component auth section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ComponentProcessorResult{
				ComponentVars:     make(map[string]any),
				ComponentSettings: make(map[string]any),
				ComponentEnv:      make(map[string]any),
			}

			err := extractComponentSections(&tt.opts, result)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedVars != nil {
				assert.Equal(t, tt.expectedVars, result.ComponentVars)
			}

			if tt.expectedSettings != nil {
				assert.Equal(t, tt.expectedSettings, result.ComponentSettings)
			}

			if tt.expectedEnv != nil {
				assert.Equal(t, tt.expectedEnv, result.ComponentEnv)
			}

			if tt.expectedAuth != nil {
				assert.Equal(t, tt.expectedAuth, result.ComponentAuth)
			}

			if tt.expectedProviders != nil {
				assert.Equal(t, tt.expectedProviders, result.ComponentProviders)
			}

			if tt.expectedHooks != nil {
				assert.Equal(t, tt.expectedHooks, result.ComponentHooks)
			}

			if tt.expectedBackend != nil {
				assert.Equal(t, tt.expectedBackend, result.ComponentBackendSection)
			}
		})
	}
}

func TestProcessComponentOverrides(t *testing.T) {
	tests := []struct {
		name                    string
		opts                    ComponentProcessorOptions
		expectedError           string
		expectedOverridesVars   map[string]any
		expectedOverridesEnv    map[string]any
		expectedOverridesCmd    string
		expectedOverridesHooks  map[string]any
		expectedOverridesExists bool
	}{
		{
			name: "process overrides for terraform component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.OverridesSectionName: map[string]any{
						cfg.VarsSectionName: map[string]any{
							"region": "us-west-2",
						},
						cfg.EnvSectionName: map[string]any{
							"AWS_REGION": "us-west-2",
						},
						cfg.CommandSectionName: "tofu",
						cfg.HooksSectionName: map[string]any{
							"before": []any{"echo override"},
						},
					},
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedOverridesVars: map[string]any{
				"region": "us-west-2",
			},
			expectedOverridesEnv: map[string]any{
				"AWS_REGION": "us-west-2",
			},
			expectedOverridesCmd: "tofu",
			expectedOverridesHooks: map[string]any{
				"before": []any{"echo override"},
			},
			expectedOverridesExists: true,
		},
		{
			name: "no overrides section",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap:  map[string]any{},
				AtmosConfig:   &schema.AtmosConfiguration{},
			},
			expectedOverridesExists: false,
		},
		{
			name: "invalid overrides section type",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.OverridesSectionName: "invalid-string",
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component overrides section",
		},
		{
			name: "invalid overrides vars section type",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.OverridesSectionName: map[string]any{
						cfg.VarsSectionName: "invalid-string",
					},
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component overrides vars section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ComponentProcessorResult{}

			err := processComponentOverrides(&tt.opts, result)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			//nolint:nestif // Test validation with multiple conditional checks.
			if tt.expectedOverridesExists {
				require.NotNil(t, result.ComponentOverrides)

				if tt.expectedOverridesVars != nil {
					assert.Equal(t, tt.expectedOverridesVars, result.ComponentOverridesVars)
				}

				if tt.expectedOverridesEnv != nil {
					assert.Equal(t, tt.expectedOverridesEnv, result.ComponentOverridesEnv)
				}

				if tt.expectedOverridesCmd != "" {
					assert.Equal(t, tt.expectedOverridesCmd, result.ComponentOverridesCommand)
				}

				if tt.expectedOverridesHooks != nil {
					assert.Equal(t, tt.expectedOverridesHooks, result.ComponentOverridesHooks)
				}
			} else {
				assert.Empty(t, result.ComponentOverrides)
			}
		})
	}
}

func TestProcessComponentInheritance(t *testing.T) {
	tests := []struct {
		name                 string
		opts                 ComponentProcessorOptions
		expectedError        string
		expectedBaseComps    []string
		expectedInheritChain []string
	}{
		{
			name: "component with single base component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.ComponentSectionName: "base-vpc",
				},
				AllComponentsMap: map[string]any{
					"base-vpc": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
				},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedBaseComps: []string{"base-vpc"},
		},
		{
			name: "component with metadata.inherits",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.MetadataSectionName: map[string]any{
						cfg.InheritsSectionName: []any{"mixin1", "mixin2"},
					},
				},
				AllComponentsMap: map[string]any{
					"mixin1": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"var1": "value1",
						},
					},
					"mixin2": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"var2": "value2",
						},
					},
				},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedBaseComps: []string{"mixin1", "mixin2"}, // Empty string no longer added.
		},
		{
			name: "invalid component attribute type",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.ComponentSectionName: 123, // Invalid: should be string
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component attribute",
		},
		{
			name: "invalid metadata.inherits type",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.MetadataSectionName: map[string]any{
						cfg.InheritsSectionName: "invalid-string", // Invalid: should be array
					},
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component metadata.inherits section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ComponentProcessorResult{
				ComponentMetadata: make(map[string]any),
			}

			// Extract metadata first if it exists.
			if metadata, ok := tt.opts.ComponentMap[cfg.MetadataSectionName]; ok {
				result.ComponentMetadata, _ = metadata.(map[string]any)
			}

			err := processComponentInheritance(&tt.opts, result)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedBaseComps != nil {
				assert.ElementsMatch(t, tt.expectedBaseComps, result.BaseComponents)
			}

			if tt.expectedInheritChain != nil {
				assert.Equal(t, tt.expectedInheritChain, result.ComponentInheritanceChain)
			}
		})
	}
}

func TestProcessTopLevelComponentInheritance(t *testing.T) {
	tests := []struct {
		name              string
		opts              ComponentProcessorOptions
		expectedError     string
		expectedBaseComps []string
	}{
		{
			name: "component with valid base component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				ComponentMap: map[string]any{
					cfg.ComponentSectionName: "base-vpc",
				},
				AllComponentsMap: map[string]any{
					"base-vpc": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
				},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedBaseComps: []string{"base-vpc"},
		},
		{
			name: "no component attribute",
			opts: ComponentProcessorOptions{
				ComponentType:    cfg.TerraformComponentType,
				Component:        "vpc",
				Stack:            "test-stack",
				StackName:        "test-stack",
				ComponentMap:     map[string]any{},
				AtmosConfig:      &schema.AtmosConfiguration{},
				AllComponentsMap: map[string]any{},
			},
			expectedBaseComps: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ComponentProcessorResult{
				BaseComponents: []string{},
			}
			baseComponentConfig := &schema.BaseComponentConfig{}
			componentInheritanceChain := []string{}

			err := processTopLevelComponentInheritance(&tt.opts, result, baseComponentConfig, &componentInheritanceChain)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedBaseComps != nil {
				assert.ElementsMatch(t, tt.expectedBaseComps, result.BaseComponents)
			}
		})
	}
}

func TestProcessMetadataInheritance(t *testing.T) {
	tests := []struct {
		name              string
		componentMetadata map[string]any
		allComponentsMap  map[string]any
		opts              ComponentProcessorOptions
		expectedError     string
		expectedBaseComps []string
	}{
		{
			name: "metadata with valid inherits list",
			componentMetadata: map[string]any{
				cfg.InheritsSectionName: []any{"mixin1", "mixin2"},
			},
			allComponentsMap: map[string]any{
				"mixin1": map[string]any{
					cfg.VarsSectionName: map[string]any{"var1": "value1"},
				},
				"mixin2": map[string]any{
					cfg.VarsSectionName: map[string]any{"var2": "value2"},
				},
			},
			opts: ComponentProcessorOptions{
				ComponentType:            cfg.TerraformComponentType,
				Component:                "derived",
				Stack:                    "test-stack",
				StackName:                "test-stack",
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			expectedBaseComps: []string{"mixin1", "mixin2"}, // Empty string no longer added.
		},
		{
			name:              "no metadata.inherits",
			componentMetadata: map[string]any{},
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				AtmosConfig:   &schema.AtmosConfiguration{},
			},
			expectedBaseComps: []string{}, // No base components when nothing is specified.
		},
		{
			name: "invalid inherits item type",
			componentMetadata: map[string]any{
				cfg.InheritsSectionName: []any{123}, // Invalid: should be string
			},
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived",
				Stack:         "test-stack",
				StackName:     "test-stack",
				AtmosConfig:   &schema.AtmosConfiguration{},
			},
			expectedError: "invalid component metadata.inherits section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ComponentProcessorResult{
				ComponentMetadata: tt.componentMetadata,
				BaseComponents:    []string{},
			}
			baseComponentConfig := &schema.BaseComponentConfig{}
			componentInheritanceChain := []string{}

			tt.opts.AllComponentsMap = tt.allComponentsMap

			err := processMetadataInheritance(&tt.opts, result, baseComponentConfig, &componentInheritanceChain)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedBaseComps != nil {
				assert.ElementsMatch(t, tt.expectedBaseComps, result.BaseComponents)
			}
		})
	}
}

func TestApplyBaseComponentConfig(t *testing.T) {
	baseComponentConfig := &schema.BaseComponentConfig{
		FinalBaseComponentName: "base-vpc",
		BaseComponentVars: map[string]any{
			"cidr": "10.0.0.0/16",
		},
		BaseComponentSettings: map[string]any{
			"enabled": true,
		},
		BaseComponentEnv: map[string]any{
			"AWS_REGION": "us-east-1",
		},
		BaseComponentCommand: "terraform",
		BaseComponentProviders: map[string]any{
			"aws": map[string]any{"region": "us-east-1"},
		},
		BaseComponentHooks: map[string]any{
			"before": []string{"echo test"},
		},
		BaseComponentBackendType: "s3",
		BaseComponentBackendSection: map[string]any{
			"bucket": "test-bucket",
		},
		BaseComponentRemoteStateBackendType: "s3",
		BaseComponentRemoteStateBackendSection: map[string]any{
			"bucket": "test-state-bucket",
		},
		ComponentInheritanceChain: []string{"base-vpc"},
	}

	opts := &ComponentProcessorOptions{
		ComponentType: cfg.TerraformComponentType,
	}

	result := &ComponentProcessorResult{}
	componentInheritanceChain := []string{}

	applyBaseComponentConfig(opts, result, baseComponentConfig, &componentInheritanceChain)

	assert.Equal(t, "base-vpc", result.BaseComponentName)
	assert.Equal(t, baseComponentConfig.BaseComponentVars, result.BaseComponentVars)
	assert.Equal(t, baseComponentConfig.BaseComponentSettings, result.BaseComponentSettings)
	assert.Equal(t, baseComponentConfig.BaseComponentEnv, result.BaseComponentEnv)
	assert.Equal(t, "terraform", result.BaseComponentCommand)
	assert.Equal(t, baseComponentConfig.BaseComponentProviders, result.BaseComponentProviders)
	assert.Equal(t, baseComponentConfig.BaseComponentHooks, result.BaseComponentHooks)
	assert.Equal(t, "s3", result.BaseComponentBackendType)
	assert.Equal(t, baseComponentConfig.BaseComponentBackendSection, result.BaseComponentBackendSection)
	assert.Equal(t, "s3", result.BaseComponentRemoteStateBackendType)
	assert.Equal(t, baseComponentConfig.BaseComponentRemoteStateBackendSection, result.BaseComponentRemoteStateBackendSection)
	assert.Equal(t, []string{"base-vpc"}, componentInheritanceChain)
}

func TestProcessInheritedComponent(t *testing.T) {
	tests := []struct {
		name              string
		opts              ComponentProcessorOptions
		result            ComponentProcessorResult
		inheritValue      any
		expectedError     string
		expectedBaseComps []string
	}{
		{
			name: "valid inherited component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				AllComponentsMap: map[string]any{
					"base-vpc": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"cidr": "10.0.0.0/16",
						},
					},
				},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			result: ComponentProcessorResult{
				BaseComponents: []string{},
			},
			inheritValue:      "base-vpc",
			expectedBaseComps: []string{"base-vpc"},
		},
		{
			name: "invalid inherit value type - not a string",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				AtmosConfig:   &schema.AtmosConfiguration{},
			},
			result: ComponentProcessorResult{
				BaseComponents: []string{},
			},
			inheritValue:  123, // Invalid: should be string
			expectedError: "invalid component metadata.inherits section",
		},
		{
			name: "component not found - CheckBaseComponentExists false",
			opts: ComponentProcessorOptions{
				ComponentType:            cfg.TerraformComponentType,
				Component:                "derived-vpc",
				Stack:                    "test-stack",
				StackName:                "test-stack",
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			result: ComponentProcessorResult{
				BaseComponents: []string{},
			},
			inheritValue:      "nonexistent-component",
			expectedBaseComps: []string{"nonexistent-component"},
		},
		{
			name: "component not found - CheckBaseComponentExists true",
			opts: ComponentProcessorOptions{
				ComponentType:            cfg.TerraformComponentType,
				Component:                "derived-vpc",
				Stack:                    "test-stack",
				StackName:                "test-stack",
				AllComponentsMap:         map[string]any{},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: true,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			result: ComponentProcessorResult{
				BaseComponents: []string{},
			},
			inheritValue:  "nonexistent-component",
			expectedError: "component not defined in any config files",
		},
		{
			name: "multiple inherited components processing",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "derived-vpc",
				Stack:         "test-stack",
				StackName:     "test-stack",
				AllComponentsMap: map[string]any{
					"mixin1": map[string]any{
						cfg.VarsSectionName: map[string]any{
							"var1": "value1",
						},
					},
				},
				ComponentsBasePath:       "/test/components",
				CheckBaseComponentExists: false,
				AtmosConfig:              &schema.AtmosConfiguration{},
			},
			result: ComponentProcessorResult{
				BaseComponents: []string{"base-vpc"},
			},
			inheritValue:      "mixin1",
			expectedBaseComps: []string{"base-vpc", "mixin1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseComponentConfig := &schema.BaseComponentConfig{}
			componentInheritanceChain := []string{}

			err := processInheritedComponent(&tt.opts, &tt.result, baseComponentConfig, &componentInheritanceChain, tt.inheritValue)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.expectedBaseComps != nil {
				assert.ElementsMatch(t, tt.expectedBaseComps, tt.result.BaseComponents)
			}
		})
	}
}
