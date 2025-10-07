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
					cfg.CommandSectionName: "tofu",
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
			expectedError: "invalid 'components.terraform.vpc.vars' section",
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
			expectedError: "invalid 'components.terraform.vpc.settings' section",
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
			expectedError: "invalid 'components.terraform.vpc.settings.spacelift' section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processComponent(tt.opts)

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

func TestProcessTerraformBackend(t *testing.T) {
	tests := []struct {
		name                         string
		component                    string
		baseComponentName            string
		globalBackendType            string
		globalBackendSection         map[string]any
		baseComponentBackendType     string
		baseComponentBackendSection  map[string]any
		componentBackendType         string
		componentBackendSection      map[string]any
		expectedBackendType          string
		expectedBackendConfig        map[string]any
		expectedWorkspaceKeyPrefix   string // For S3
		expectedPrefix               string // For GCS
		expectedKey                  string // For Azure
	}{
		{
			name:              "s3 backend with default workspace_key_prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
					"region": "us-east-1",
				},
			},
			expectedBackendType: "s3",
			expectedBackendConfig: map[string]any{
				"bucket":               "test-bucket",
				"region":               "us-east-1",
				"workspace_key_prefix": "vpc",
			},
			expectedWorkspaceKeyPrefix: "vpc",
		},
		{
			name:              "s3 backend with base component name",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "s3",
			expectedWorkspaceKeyPrefix: "base-vpc",
		},
		{
			name:              "s3 backend with existing workspace_key_prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket":               "test-bucket",
					"workspace_key_prefix": "custom-prefix",
				},
			},
			expectedBackendType:        "s3",
			expectedWorkspaceKeyPrefix: "custom-prefix",
		},
		{
			name:              "gcs backend with default prefix",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "gcs",
			globalBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "gcs",
			expectedPrefix:      "vpc",
		},
		{
			name:              "gcs backend with base component name",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "gcs",
			globalBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "gcs",
			expectedPrefix:      "base-vpc",
		},
		{
			name:              "azurerm backend with default key",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
					"key":                  "global",
				},
			},
			componentBackendSection: map[string]any{},
			expectedBackendType:     "azurerm",
			expectedKey:             "global/vpc.terraform.tfstate",
		},
		{
			name:              "azurerm backend with base component name",
			component:         "derived-vpc",
			baseComponentName: "base-vpc",
			globalBackendType: "azurerm",
			globalBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
					"container_name":       "tfstate",
					"key":                  "global",
				},
			},
			componentBackendSection: map[string]any{},
			expectedBackendType:     "azurerm",
			expectedKey:             "global/base-vpc.terraform.tfstate",
		},
		{
			name:              "backend type precedence - component overrides base",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			baseComponentBackendType: "gcs",
			componentBackendType:     "azurerm",
			componentBackendSection: map[string]any{
				"azurerm": map[string]any{
					"storage_account_name": "test-account",
				},
			},
			expectedBackendType: "azurerm",
		},
		{
			name:              "backend type precedence - base overrides global",
			component:         "vpc",
			baseComponentName: "",
			globalBackendType: "s3",
			baseComponentBackendType: "gcs",
			globalBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType: "gcs",
		},
		{
			name:                     "component with slashes in name",
			component:                "path/to/vpc",
			baseComponentName:        "",
			globalBackendType:        "s3",
			globalBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedBackendType:        "s3",
			expectedWorkspaceKeyPrefix: "path-to-vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			backendType, backendConfig, err := processTerraformBackend(
				atmosConfig,
				tt.component,
				tt.baseComponentName,
				tt.globalBackendType,
				tt.globalBackendSection,
				tt.baseComponentBackendType,
				tt.baseComponentBackendSection,
				tt.componentBackendType,
				tt.componentBackendSection,
			)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedBackendType, backendType)

			if tt.expectedWorkspaceKeyPrefix != "" {
				assert.Equal(t, tt.expectedWorkspaceKeyPrefix, backendConfig["workspace_key_prefix"])
			}

			if tt.expectedPrefix != "" {
				assert.Equal(t, tt.expectedPrefix, backendConfig["prefix"])
			}

			if tt.expectedKey != "" {
				assert.Equal(t, tt.expectedKey, backendConfig["key"])
			}

			if tt.expectedBackendConfig != nil {
				for key, expectedValue := range tt.expectedBackendConfig {
					assert.Equal(t, expectedValue, backendConfig[key])
				}
			}
		})
	}
}

func TestProcessTerraformRemoteStateBackend(t *testing.T) {
	tests := []struct {
		name                                    string
		component                               string
		finalComponentBackendType               string
		finalComponentBackendSection            map[string]any
		globalRemoteStateBackendType            string
		globalRemoteStateBackendSection         map[string]any
		baseComponentRemoteStateBackendType     string
		baseComponentRemoteStateBackendSection  map[string]any
		componentRemoteStateBackendType         string
		componentRemoteStateBackendSection      map[string]any
		expectedRemoteStateBackendType          string
		expectedRemoteStateBackendConfigNotNil  bool
	}{
		{
			name:                      "remote state backend inherits from backend type",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedRemoteStateBackendType: "s3",
			expectedRemoteStateBackendConfigNotNil: true,
		},
		{
			name:                      "global remote state backend type overrides backend type",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			globalRemoteStateBackendType: "gcs",
			globalRemoteStateBackendSection: map[string]any{
				"gcs": map[string]any{
					"bucket": "remote-state-bucket",
				},
			},
			expectedRemoteStateBackendType: "gcs",
			expectedRemoteStateBackendConfigNotNil: true,
		},
		{
			name:                      "component remote state backend type takes precedence",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "test-bucket",
				},
			},
			globalRemoteStateBackendType: "gcs",
			baseComponentRemoteStateBackendType: "azurerm",
			componentRemoteStateBackendType:     "s3",
			componentRemoteStateBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "component-remote-state",
				},
			},
			expectedRemoteStateBackendType: "s3",
			expectedRemoteStateBackendConfigNotNil: true,
		},
		{
			name:                      "merges backend and remote_state_backend sections",
			component:                 "vpc",
			finalComponentBackendType: "s3",
			finalComponentBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "backend-bucket",
					"region": "us-east-1",
				},
			},
			componentRemoteStateBackendSection: map[string]any{
				"s3": map[string]any{
					"bucket": "remote-state-bucket",
				},
			},
			expectedRemoteStateBackendType: "s3",
			expectedRemoteStateBackendConfigNotNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			remoteStateBackendType, remoteStateBackendConfig, err := processTerraformRemoteStateBackend(
				atmosConfig,
				tt.component,
				tt.finalComponentBackendType,
				tt.finalComponentBackendSection,
				tt.globalRemoteStateBackendType,
				tt.globalRemoteStateBackendSection,
				tt.baseComponentRemoteStateBackendType,
				tt.baseComponentRemoteStateBackendSection,
				tt.componentRemoteStateBackendType,
				tt.componentRemoteStateBackendSection,
			)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedRemoteStateBackendType, remoteStateBackendType)

			if tt.expectedRemoteStateBackendConfigNotNil {
				assert.NotNil(t, remoteStateBackendConfig)
			}
		})
	}
}

func TestMergeComponentConfigurations(t *testing.T) {
	tests := []struct {
		name                  string
		opts                  ComponentProcessorOptions
		result                *ComponentProcessorResult
		expectedVars          map[string]any
		expectedSettings      map[string]any
		expectedEnv           map[string]any
		expectedCommand       string
		expectedProviders     map[string]any
		expectedHooks         map[string]any
		checkBaseComponent    bool
		expectedBaseComponent string
	}{
		{
			name: "terraform component with all fields",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.TerraformComponentType,
				Component:     "vpc",
				GlobalVars: map[string]any{
					"global_var": "global",
				},
				GlobalSettings: map[string]any{
					"global_setting": true,
				},
				GlobalEnv: map[string]any{
					"GLOBAL_ENV": "value",
				},
				GlobalCommand: "terraform",
				TerraformProviders: map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
				},
				GlobalAndTerraformHooks: map[string]any{
					"before": []string{"global hook"},
				},
				GlobalBackendType: "s3",
				GlobalBackendSection: map[string]any{
					"s3": map[string]any{
						"bucket": "test-bucket",
					},
				},
				AtmosConfig: &schema.AtmosConfiguration{},
			},
			result: &ComponentProcessorResult{
				ComponentVars: map[string]any{
					"component_var": "component",
				},
				ComponentSettings: map[string]any{
					"component_setting": false,
				},
				ComponentEnv: map[string]any{
					"COMPONENT_ENV": "value",
				},
				ComponentCommand: "tofu",
				ComponentMetadata: map[string]any{
					"type": "real",
				},
				ComponentOverrides:        map[string]any{},
				ComponentOverridesVars:    map[string]any{},
				ComponentOverridesSettings: map[string]any{},
				ComponentOverridesEnv:     map[string]any{},
				BaseComponentVars:         map[string]any{},
				BaseComponentSettings:     map[string]any{},
				BaseComponentEnv:          map[string]any{},
				ComponentProviders: map[string]any{
					"aws": map[string]any{
						"profile": "test",
					},
				},
				ComponentHooks: map[string]any{
					"after": []string{"component hook"},
				},
				ComponentAuth:                          map[string]any{},
				ComponentBackendType:                   "",
				ComponentBackendSection:                map[string]any{},
				ComponentRemoteStateBackendType:        "",
				ComponentRemoteStateBackendSection:     map[string]any{},
				ComponentOverridesProviders:            map[string]any{},
				ComponentOverridesHooks:                map[string]any{},
				BaseComponentProviders:                 map[string]any{},
				BaseComponentHooks:                     map[string]any{},
				BaseComponentBackendType:               "",
				BaseComponentBackendSection:            map[string]any{},
				BaseComponentRemoteStateBackendType:    "",
				BaseComponentRemoteStateBackendSection: map[string]any{},
			},
			expectedVars: map[string]any{
				"global_var":    "global",
				"component_var": "component",
			},
			expectedSettings: map[string]any{
				"global_setting":    true,
				"component_setting": false,
			},
			expectedEnv: map[string]any{
				"GLOBAL_ENV":    "value",
				"COMPONENT_ENV": "value",
			},
			expectedCommand: "tofu",
		},
		{
			name: "helmfile component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.HelmfileComponentType,
				Component:     "app",
				GlobalVars: map[string]any{
					"namespace": "kube-system",
				},
				GlobalSettings: map[string]any{
					"enabled": true,
				},
				GlobalEnv:   map[string]any{},
				AtmosConfig: &schema.AtmosConfiguration{
					Components: schema.Components{
						Helmfile: schema.Helmfile{
							Command: "helmfile",
						},
					},
				},
			},
			result: &ComponentProcessorResult{
				ComponentVars: map[string]any{
					"namespace": "default",
				},
				ComponentSettings:          map[string]any{},
				ComponentEnv:               map[string]any{},
				ComponentMetadata:          map[string]any{},
				ComponentOverrides:         map[string]any{},
				ComponentOverridesVars:     map[string]any{},
				ComponentOverridesSettings: map[string]any{},
				ComponentOverridesEnv:      map[string]any{},
				BaseComponentVars:          map[string]any{},
				BaseComponentSettings:      map[string]any{},
				BaseComponentEnv:           map[string]any{},
			},
			expectedVars: map[string]any{
				"namespace": "default",
			},
			expectedCommand: "helmfile",
		},
		{
			name: "packer component",
			opts: ComponentProcessorOptions{
				ComponentType: cfg.PackerComponentType,
				Component:     "ami",
				GlobalVars: map[string]any{
					"region": "us-east-1",
				},
				GlobalSettings: map[string]any{},
				GlobalEnv:      map[string]any{},
				AtmosConfig:    &schema.AtmosConfiguration{},
			},
			result: &ComponentProcessorResult{
				ComponentVars: map[string]any{
					"ami_name": "test-ami",
				},
				ComponentSettings:          map[string]any{},
				ComponentEnv:               map[string]any{},
				ComponentMetadata:          map[string]any{},
				ComponentOverrides:         map[string]any{},
				ComponentOverridesVars:     map[string]any{},
				ComponentOverridesSettings: map[string]any{},
				ComponentOverridesEnv:      map[string]any{},
				BaseComponentVars:          map[string]any{},
				BaseComponentSettings:      map[string]any{},
				BaseComponentEnv:           map[string]any{},
			},
			expectedVars: map[string]any{
				"region":   "us-east-1",
				"ami_name": "test-ami",
			},
			expectedCommand: cfg.PackerComponentType,
		},
		{
			name: "component with base component name",
			opts: ComponentProcessorOptions{
				ComponentType:  cfg.TerraformComponentType,
				Component:      "derived-vpc",
				GlobalVars:     map[string]any{},
				GlobalSettings: map[string]any{},
				GlobalEnv:      map[string]any{},
				AtmosConfig:    &schema.AtmosConfiguration{},
			},
			result: &ComponentProcessorResult{
				ComponentVars:              map[string]any{},
				ComponentSettings:          map[string]any{},
				ComponentEnv:               map[string]any{},
				ComponentMetadata:          map[string]any{},
				ComponentOverrides:         map[string]any{},
				ComponentOverridesVars:     map[string]any{},
				ComponentOverridesSettings: map[string]any{},
				ComponentOverridesEnv:      map[string]any{},
				BaseComponentName:          "base-vpc",
				BaseComponentVars:          map[string]any{},
				BaseComponentSettings:      map[string]any{},
				BaseComponentEnv:           map[string]any{},
				ComponentProviders:                     map[string]any{},
				ComponentHooks:                         map[string]any{},
				ComponentAuth:                          map[string]any{},
				ComponentBackendType:                   "",
				ComponentBackendSection:                map[string]any{},
				ComponentRemoteStateBackendType:        "",
				ComponentRemoteStateBackendSection:     map[string]any{},
				ComponentOverridesProviders:            map[string]any{},
				ComponentOverridesHooks:                map[string]any{},
				BaseComponentProviders:                 map[string]any{},
				BaseComponentHooks:                     map[string]any{},
				BaseComponentBackendType:               "",
				BaseComponentBackendSection:            map[string]any{},
				BaseComponentRemoteStateBackendType:    "",
				BaseComponentRemoteStateBackendSection: map[string]any{},
			},
			checkBaseComponent:    true,
			expectedBaseComponent: "base-vpc",
		},
		{
			name: "terraform abstract component removes spacelift workspace_enabled",
			opts: ComponentProcessorOptions{
				ComponentType:  cfg.TerraformComponentType,
				Component:      "abstract-vpc",
				GlobalVars:     map[string]any{},
				GlobalSettings: map[string]any{},
				GlobalEnv:      map[string]any{},
				GlobalBackendType: "s3",
				GlobalBackendSection: map[string]any{
					"s3": map[string]any{
						"bucket": "test",
					},
				},
				TerraformProviders: map[string]any{},
				GlobalAndTerraformHooks: map[string]any{},
				AtmosConfig:    &schema.AtmosConfiguration{},
			},
			result: &ComponentProcessorResult{
				ComponentVars: map[string]any{},
				ComponentSettings: map[string]any{
					"spacelift": map[string]any{
						"workspace_enabled": true,
					},
				},
				ComponentEnv: map[string]any{},
				ComponentMetadata: map[string]any{
					"type": cfg.AbstractSectionName,
				},
				ComponentOverrides:                     map[string]any{},
				ComponentOverridesVars:                 map[string]any{},
				ComponentOverridesSettings:             map[string]any{},
				ComponentOverridesEnv:                  map[string]any{},
				BaseComponentVars:                      map[string]any{},
				BaseComponentSettings:                  map[string]any{},
				BaseComponentEnv:                       map[string]any{},
				ComponentProviders:                     map[string]any{},
				ComponentHooks:                         map[string]any{},
				ComponentAuth:                          map[string]any{},
				ComponentBackendType:                   "",
				ComponentBackendSection:                map[string]any{},
				ComponentRemoteStateBackendType:        "",
				ComponentRemoteStateBackendSection:     map[string]any{},
				ComponentOverridesProviders:            map[string]any{},
				ComponentOverridesHooks:                map[string]any{},
				BaseComponentProviders:                 map[string]any{},
				BaseComponentHooks:                     map[string]any{},
				BaseComponentBackendType:               "",
				BaseComponentBackendSection:            map[string]any{},
				BaseComponentRemoteStateBackendType:    "",
				BaseComponentRemoteStateBackendSection: map[string]any{},
			},
			expectedSettings: map[string]any{
				"spacelift": map[string]any{
					// workspace_enabled should be removed
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := mergeComponentConfigurations(tt.opts.AtmosConfig, tt.opts, tt.result)

			require.NoError(t, err)
			require.NotNil(t, comp)

			if tt.expectedVars != nil {
				actualVars := comp[cfg.VarsSectionName].(map[string]any)
				assert.Equal(t, tt.expectedVars, actualVars)
			}

			if tt.expectedSettings != nil {
				actualSettings := comp[cfg.SettingsSectionName].(map[string]any)
				for key, expectedValue := range tt.expectedSettings {
					assert.Equal(t, expectedValue, actualSettings[key])
				}
			}

			if tt.expectedEnv != nil {
				actualEnv := comp[cfg.EnvSectionName].(map[string]any)
				assert.Equal(t, tt.expectedEnv, actualEnv)
			}

			if tt.expectedCommand != "" {
				assert.Equal(t, tt.expectedCommand, comp[cfg.CommandSectionName])
			}

			if tt.expectedProviders != nil {
				actualProviders := comp[cfg.ProvidersSectionName].(map[string]any)
				for key, expectedValue := range tt.expectedProviders {
					assert.Equal(t, expectedValue, actualProviders[key])
				}
			}

			if tt.expectedHooks != nil {
				actualHooks := comp[cfg.HooksSectionName].(map[string]any)
				for key, expectedValue := range tt.expectedHooks {
					assert.Equal(t, expectedValue, actualHooks[key])
				}
			}

			if tt.checkBaseComponent {
				assert.Equal(t, tt.expectedBaseComponent, comp[cfg.ComponentSectionName])
			}
		})
	}
}

func TestProcessAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		authConfig  map[string]any
		globalAuth  schema.AuthConfig
		expected    map[string]any
		expectError bool
	}{
		{
			name: "merge component auth with empty global auth",
			authConfig: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"enabled": true,
					},
				},
			},
			globalAuth: schema.AuthConfig{
				Providers: map[string]schema.Provider{},
			},
			expected: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"enabled": true,
					},
				},
			},
			expectError: false,
		},
		{
			name:       "empty component auth uses global auth",
			authConfig: map[string]any{},
			globalAuth: schema.AuthConfig{
				Providers: map[string]schema.Provider{},
			},
			expected:    map[string]any{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Auth: tt.globalAuth,
			}

			result, err := processAuthConfig(atmosConfig, tt.authConfig)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}
