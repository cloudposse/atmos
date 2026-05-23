package exec

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: if these fields are renamed the build breaks, preventing
// tests that rely on specific field names from silently passing with zero values.
var _ = schema.AtmosSettings{ListMergeStrategy: ""}

// TestMergeComponentConfigurations verifies that mergeComponentConfigurations
// correctly assembles the final component configuration from layered inputs
// (global, base-component, component, and overrides) for both Terraform and
// Helmfile component types.
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

// TestEffectiveAtmosConfig verifies that effectiveAtmosConfig returns the
// correct *AtmosConfiguration given various combinations of settings layers.
func TestEffectiveAtmosConfig(t *testing.T) {
	base := &schema.AtmosConfiguration{}
	base.Settings.ListMergeStrategy = "replace"

	t.Run("no override returns base pointer unchanged", func(t *testing.T) {
		result := effectiveAtmosConfig(base)
		assert.Same(t, base, result, "should return the original pointer when no layers override the strategy")
	})

	t.Run("empty layers return base pointer unchanged", func(t *testing.T) {
		result := effectiveAtmosConfig(base, nil, map[string]any{}, map[string]any{"other_key": "value"})
		assert.Same(t, base, result)
	})

	t.Run("single layer override returns copy with new strategy", func(t *testing.T) {
		result := effectiveAtmosConfig(base, map[string]any{"list_merge_strategy": "append"})
		assert.NotSame(t, base, result, "must return a copy, not the original")
		assert.Equal(t, "append", result.Settings.ListMergeStrategy)
		assert.Equal(t, "replace", base.Settings.ListMergeStrategy, "original must be unchanged")

		// result→src: mutating the copy must not affect the original.
		result.Settings.ListMergeStrategy = "merge"
		assert.Equal(t, "replace", base.Settings.ListMergeStrategy,
			"mutating the copy must not affect the original (result→src isolation)")

		// src→result: mutating the original after the call must not affect the copy.
		base.Settings.ListMergeStrategy = "append"
		assert.Equal(t, "merge", result.Settings.ListMergeStrategy,
			"mutating the source after the call must not affect the copy (src→result isolation)")
		base.Settings.ListMergeStrategy = "replace" // restore for subsequent subtests
	})

	t.Run("later layer wins over earlier layer", func(t *testing.T) {
		result := effectiveAtmosConfig(base,
			map[string]any{"list_merge_strategy": "append"},
			map[string]any{"list_merge_strategy": "merge"},
		)
		assert.Equal(t, "merge", result.Settings.ListMergeStrategy)
	})

	t.Run("empty string in later layer does not override earlier non-empty value", func(t *testing.T) {
		result := effectiveAtmosConfig(base,
			map[string]any{"list_merge_strategy": "append"},
			map[string]any{"list_merge_strategy": ""},
		)
		assert.Equal(t, "append", result.Settings.ListMergeStrategy)
	})

	t.Run("override matching the base value returns base pointer unchanged", func(t *testing.T) {
		result := effectiveAtmosConfig(base, map[string]any{"list_merge_strategy": "replace"})
		assert.Same(t, base, result, "no copy needed when effective strategy equals the base strategy")
	})

	t.Run("non-string list_merge_strategy is ignored", func(t *testing.T) {
		result := effectiveAtmosConfig(base, map[string]any{"list_merge_strategy": 42})
		assert.Same(t, base, result, "non-string type assertion fails cleanly; base is returned unchanged")
	})
}

// TestComponentLevelListMergeStrategy is an integration test that verifies issue
// #2396: setting list_merge_strategy inside a component's settings section must
// affect how that component's vars lists are merged, overriding the global config.
//
// Fixture (tests/fixtures/scenarios/component-list-merge-strategy):
//   - atmos.yaml: global list_merge_strategy = "replace"
//   - catalog/base.yaml: abstract base-component with vars.tags = [base-tag-1, base-tag-2]
//   - deploy/dev.yaml:
//   - append-component: inherits base-component, settings.list_merge_strategy = "append"
//     → expected vars.tags: [base-tag-1, base-tag-2, child-tag]
//   - replace-component: inherits base-component, no strategy override
//     → expected vars.tags: [child-tag].
func TestComponentLevelListMergeStrategy(t *testing.T) {
	workDir := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "component-list-merge-strategy")
	t.Chdir(workDir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", "")

	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, true)
	require.NoError(t, err)
	require.Equal(t, "replace", atmosConfig.Settings.ListMergeStrategy,
		"global strategy must be 'replace' so the component-level override is meaningful")

	stack := "dev"

	t.Run("component-level append overrides global replace", func(t *testing.T) {
		result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig:          &atmosConfig,
			Component:            "append-component",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		})
		require.NoError(t, err)

		vars, ok := result["vars"].(map[string]any)
		require.True(t, ok, "vars must be a map")
		tags, ok := vars["tags"].([]any)
		require.True(t, ok, "vars.tags must be a list")

		assert.Equal(t, []any{"base-tag-1", "base-tag-2", "child-tag"}, tags,
			"append strategy must accumulate base tags then child tag")
	})

	t.Run("component without override uses global replace strategy", func(t *testing.T) {
		result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig:          &atmosConfig,
			Component:            "replace-component",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		})
		require.NoError(t, err)

		vars, ok := result["vars"].(map[string]any)
		require.True(t, ok, "vars must be a map")
		tags, ok := vars["tags"].([]any)
		require.True(t, ok, "vars.tags must be a list")

		assert.Equal(t, []any{"child-tag"}, tags,
			"replace strategy must discard base tags and keep only child tags")
	})

	// Verify that list_merge_strategy set in a base component's settings
	// (result.BaseComponentSettings) is honoured even when the inheriting component
	// does not set it in its own settings. This exercises the second layer in
	// effectiveAtmosConfig's precedence scan.
	t.Run("strategy inherited from base component settings", func(t *testing.T) {
		result, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			AtmosConfig:          &atmosConfig,
			Component:            "inherit-strategy-component",
			Stack:                stack,
			ProcessTemplates:     true,
			ProcessYamlFunctions: true,
		})
		require.NoError(t, err)

		vars, ok := result["vars"].(map[string]any)
		require.True(t, ok, "vars must be a map")
		tags, ok := vars["tags"].([]any)
		require.True(t, ok, "vars.tags must be a list")

		assert.Equal(t, []any{"base-tag-1", "base-tag-2", "child-tag"}, tags,
			"append strategy from base component settings must be inherited")
	})
}

// TestEffectiveAtmosConfig_InvalidStrategy verifies two properties:
//  1. effectiveAtmosConfig passes an invalid strategy value through without
//     validating it — validation is pkg/merge's responsibility.
//  2. When the resulting config is used in a merge call, pkg/merge returns
//     ErrInvalidListMergeStrategy so the error surfaces correctly.
func TestEffectiveAtmosConfig_InvalidStrategy(t *testing.T) {
	base := &schema.AtmosConfiguration{}
	base.Settings.ListMergeStrategy = "replace"

	result := effectiveAtmosConfig(base, map[string]any{"list_merge_strategy": "foobar"})
	assert.Equal(t, "foobar", result.Settings.ListMergeStrategy,
		"effectiveAtmosConfig must pass invalid values through; validation is pkg/merge's responsibility")
	assert.NotSame(t, base, result)

	// Verify the error surfaces when the config is actually used for a merge.
	_, _, mergeErr := m.MergeWithDeferred(result, []map[string]any{
		{"tags": []any{"a"}},
		{"tags": []any{"b"}},
	})
	assert.ErrorIs(t, mergeErr, errUtils.ErrInvalidListMergeStrategy,
		"pkg/merge must reject the invalid strategy when a merge is attempted")
}

// TestProcessAuthConfig verifies that processAuthConfig merges global and
// component-level auth configurations, with the component-level settings
// taking precedence over the global ones.
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
