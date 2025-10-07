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
