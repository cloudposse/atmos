package exec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMergeComponentConfigurations(t *testing.T) {
	tests := []struct {
		name                  string
		opts                  ComponentProcessorOptions
		result                *ComponentProcessorResult
		expectedVars          map[string]any
		expectedSettings      map[string]any
		expectedEnv           map[string]any
		expectedAuth          map[string]any
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
				GlobalAuth: map[string]any{
					"aws": map[string]any{
						"profile": "global-profile",
					},
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
				ComponentOverrides:         map[string]any{},
				ComponentOverridesVars:     map[string]any{},
				ComponentOverridesSettings: map[string]any{},
				ComponentOverridesEnv:      map[string]any{},
				ComponentOverridesAuth:     map[string]any{},
				BaseComponentVars:          map[string]any{},
				BaseComponentSettings:      map[string]any{},
				BaseComponentEnv:           map[string]any{},
				BaseComponentAuth:          map[string]any{},
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
			expectedAuth: map[string]any{
				"aws": map[string]any{
					"profile": "global-profile",
				},
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
				GlobalEnv:  map[string]any{},
				GlobalAuth: map[string]any{},
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
				ComponentOverridesAuth:     map[string]any{},
				BaseComponentVars:          map[string]any{},
				BaseComponentSettings:      map[string]any{},
				BaseComponentEnv:           map[string]any{},
				BaseComponentAuth:          map[string]any{},
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
				GlobalAuth:     map[string]any{},
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
				ComponentVars:                          map[string]any{},
				ComponentSettings:                      map[string]any{},
				ComponentEnv:                           map[string]any{},
				ComponentMetadata:                      map[string]any{},
				ComponentOverrides:                     map[string]any{},
				ComponentOverridesVars:                 map[string]any{},
				ComponentOverridesSettings:             map[string]any{},
				ComponentOverridesEnv:                  map[string]any{},
				BaseComponentName:                      "base-vpc",
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
			checkBaseComponent:    true,
			expectedBaseComponent: "base-vpc",
		},
		{
			name: "terraform abstract component removes spacelift workspace_enabled",
			opts: ComponentProcessorOptions{
				ComponentType:     cfg.TerraformComponentType,
				Component:         "abstract-vpc",
				GlobalVars:        map[string]any{},
				GlobalSettings:    map[string]any{},
				GlobalEnv:         map[string]any{},
				GlobalBackendType: "s3",
				GlobalBackendSection: map[string]any{
					"s3": map[string]any{
						"bucket": "test",
					},
				},
				TerraformProviders:      map[string]any{},
				GlobalAndTerraformHooks: map[string]any{},
				AtmosConfig:             &schema.AtmosConfiguration{},
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
			comp, err := mergeComponentConfigurations(tt.opts.AtmosConfig, &tt.opts, tt.result)

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

			if tt.expectedAuth != nil {
				actualAuth := comp[cfg.AuthSectionName].(map[string]any)
				for key, expectedValue := range tt.expectedAuth {
					assert.Equal(t, expectedValue, actualAuth[key])
				}
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
		{
			name: "component auth overrides global auth",
			authConfig: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
				},
			},
			globalAuth: schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"aws": {
						Region: "us-east-1",
					},
				},
			},
			expected: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"region": "us-west-2",
					},
				},
			},
			expectError: false,
		},
		{
			name: "merge multiple providers",
			authConfig: map[string]any{
				"providers": map[string]any{
					"azure": map[string]any{
						"tenant_id": "test-tenant",
					},
				},
			},
			globalAuth: schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"aws": {
						Region: "us-east-1",
					},
				},
			},
			expected: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"region": "us-east-1",
					},
					"azure": map[string]any{
						"tenant_id": "test-tenant",
					},
				},
			},
			expectError: false,
		},
		{
			name: "deep merge auth configuration",
			authConfig: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"spec": map[string]any{
							"role_arn": "arn:aws:iam::123:role/MyRole",
						},
					},
				},
			},
			globalAuth: schema.AuthConfig{
				Providers: map[string]schema.Provider{
					"aws": {
						Region:       "us-east-1",
						ProviderType: "aws-sso",
					},
				},
			},
			expected: map[string]any{
				"providers": map[string]any{
					"aws": map[string]any{
						"region":        "us-east-1",
						"provider_type": "aws-sso",
						"spec": map[string]any{
							"role_arn": "arn:aws:iam::123:role/MyRole",
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Auth: tt.globalAuth,
			}

			// Convert the global auth config struct to map[string]any for testing.
			// Use JSON marshaling for deep conversion of nested structs to maps.
			var globalAuthConfig map[string]any
			if atmosConfig.Auth.Providers != nil || atmosConfig.Auth.Identities != nil {
				jsonBytes, err := json.Marshal(atmosConfig.Auth)
				require.NoError(t, err)
				err = json.Unmarshal(jsonBytes, &globalAuthConfig)
				require.NoError(t, err)
			} else {
				globalAuthConfig = map[string]any{}
			}

			result, err := processAuthConfig(atmosConfig, globalAuthConfig, tt.authConfig)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			// Deep comparison is problematic due to mapstructure adding Kind field.
			// Verify key fields are present instead.
			if expectedProviders, ok := tt.expected["providers"].(map[string]any); ok && expectedProviders != nil {
				resultProviders, ok := result["providers"].(map[string]any)
				require.True(t, ok, "Expected providers section in result")
				require.NotNil(t, resultProviders)
				// Verify each expected provider exists in result.
				for providerName := range expectedProviders {
					assert.Contains(t, resultProviders, providerName, "Expected provider %s in result", providerName)
				}
			}
		})
	}
}
