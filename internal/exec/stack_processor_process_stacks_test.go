package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessStackConfig_ErrorPaths(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name           string
		config         map[string]any
		expectedError  error
		errorSubstring string
	}{
		{
			name: "invalid vars section type",
			config: map[string]any{
				cfg.VarsSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidVarsSection,
		},
		{
			name: "invalid hooks section type",
			config: map[string]any{
				cfg.HooksSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidHooksSection,
		},
		{
			name: "invalid settings section type",
			config: map[string]any{
				cfg.SettingsSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidSettingsSection,
		},
		{
			name: "invalid env section type",
			config: map[string]any{
				cfg.EnvSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidEnvSection,
		},
		{
			name: "invalid terraform section type",
			config: map[string]any{
				cfg.TerraformSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidTerraformSection,
		},
		{
			name: "invalid helmfile section type",
			config: map[string]any{
				cfg.HelmfileSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidHelmfileSection,
		},
		{
			name: "invalid packer section type",
			config: map[string]any{
				cfg.PackerSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidPackerSection,
		},
		{
			name: "invalid components section type",
			config: map[string]any{
				cfg.ComponentsSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidComponentsSection,
		},
		{
			name: "invalid auth section type",
			config: map[string]any{
				cfg.AuthSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidAuthSection,
		},
		{
			name: "invalid terraform command type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.CommandSectionName: 123,
				},
			},
			expectedError: errUtils.ErrInvalidTerraformCommand,
		},
		{
			name: "invalid terraform vars type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.VarsSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformVars,
		},
		{
			name: "invalid terraform hooks type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.HooksSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformHooksSection,
		},
		{
			name: "invalid terraform settings type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.SettingsSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformSettings,
		},
		{
			name: "invalid terraform env type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.EnvSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformEnv,
		},
		{
			name: "invalid terraform providers type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.ProvidersSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformProviders,
		},
		{
			name: "invalid terraform auth type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.AuthSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformAuth,
		},
		{
			name: "invalid terraform backend_type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.BackendTypeSectionName: 123,
				},
			},
			expectedError: errUtils.ErrInvalidTerraformBackendType,
		},
		{
			name: "invalid terraform backend",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.BackendSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformBackend,
		},
		{
			name: "invalid terraform remote_state_backend_type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.RemoteStateBackendTypeSectionName: 123,
				},
			},
			expectedError: errUtils.ErrInvalidTerraformRemoteStateType,
		},
		{
			name: "invalid terraform remote_state_backend",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.RemoteStateBackendSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformRemoteStateSection,
		},
		{
			name: "invalid helmfile command type",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.CommandSectionName: 123,
				},
			},
			expectedError: errUtils.ErrInvalidHelmfileCommand,
		},
		{
			name: "invalid helmfile vars type",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.VarsSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidHelmfileVars,
		},
		{
			name: "invalid helmfile settings type",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.SettingsSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidHelmfileSettings,
		},
		{
			name: "invalid helmfile env type",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.EnvSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidHelmfileEnv,
		},
		{
			name: "invalid helmfile auth type",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.AuthSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidHelmfileAuth,
		},
		{
			name: "invalid packer command type",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.CommandSectionName: 123,
				},
			},
			expectedError: errUtils.ErrInvalidPackerCommand,
		},
		{
			name: "invalid packer vars type",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.VarsSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidPackerVars,
		},
		{
			name: "invalid packer settings type",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.SettingsSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidPackerSettings,
		},
		{
			name: "invalid packer env type",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.EnvSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidPackerEnv,
		},
		{
			name: "invalid packer auth type",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.AuthSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidPackerAuth,
		},
		{
			name: "invalid components.terraform type",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.TerraformComponentType: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidComponentsTerraform,
		},
		{
			name: "invalid components.helmfile type",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.HelmfileComponentType: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidComponentsHelmfile,
		},
		{
			name: "invalid components.packer type",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.PackerComponentType: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidComponentsPacker,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProcessStackConfig(
				atmosConfig,
				"/test/stacks",
				"/test/terraform",
				"/test/helmfile",
				"/test/packer",
				"test-stack.yaml",
				tt.config,
				false,
				false,
				"",
				map[string]map[string][]string{},
				map[string]map[string]any{},
				false,
			)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedError)
		})
	}
}

func TestProcessStackConfig_HappyPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name                string
		config              map[string]any
		componentTypeFilter string
		validateResult      func(t *testing.T, result map[string]any)
	}{
		{
			name:   "minimal valid config with empty components",
			config: map[string]any{},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with global vars",
			config: map[string]any{
				cfg.VarsSectionName: map[string]any{
					"environment": "dev",
					"region":      "us-east-1",
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with global settings",
			config: map[string]any{
				cfg.SettingsSectionName: map[string]any{
					"spacelift": map[string]any{
						"workspace_enabled": true,
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with global env",
			config: map[string]any{
				cfg.EnvSectionName: map[string]any{
					"AWS_REGION": "us-east-1",
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with terraform section",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"enabled": true,
					},
					cfg.SettingsSectionName: map[string]any{
						"version": "1.0.0",
					},
					cfg.EnvSectionName: map[string]any{
						"TF_LOG": "DEBUG",
					},
					cfg.CommandSectionName: "terraform",
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with helmfile section",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"namespace": "default",
					},
					cfg.SettingsSectionName: map[string]any{
						"version": "0.150.0",
					},
					cfg.EnvSectionName: map[string]any{
						"HELM_DEBUG": "true",
					},
					cfg.CommandSectionName: "helmfile",
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with packer section",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.VarsSectionName: map[string]any{
						"ami_name": "test-ami",
					},
					cfg.SettingsSectionName: map[string]any{
						"version": "1.8.0",
					},
					cfg.EnvSectionName: map[string]any{
						"PACKER_LOG": "1",
					},
					cfg.CommandSectionName: "packer",
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with stack-level name override",
			config: map[string]any{
				cfg.NameSectionName: "custom-stack-name",
				cfg.VarsSectionName: map[string]any{
					"environment": "prod",
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with terraform backend",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.BackendTypeSectionName: "s3",
					cfg.BackendSectionName: map[string]any{
						"bucket": "terraform-state",
						"region": "us-east-1",
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with terraform remote_state_backend",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.RemoteStateBackendTypeSectionName: "s3",
					cfg.RemoteStateBackendSectionName: map[string]any{
						"bucket": "remote-state",
						"region": "us-west-2",
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with terraform providers",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.ProvidersSectionName: map[string]any{
						"aws": map[string]any{
							"region": "us-east-1",
						},
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with terraform hooks",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.HooksSectionName: map[string]any{
						"before_init": []any{"echo 'Before init'"},
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with global hooks",
			config: map[string]any{
				cfg.HooksSectionName: map[string]any{
					"before_all": []any{"echo 'Hello'"},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with auth section",
			config: map[string]any{
				cfg.AuthSectionName: map[string]any{
					"default": map[string]any{
						"type": "aws",
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with terraform auth",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.AuthSectionName: map[string]any{
						"role": "admin",
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with helmfile auth",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.AuthSectionName: map[string]any{
						"role": "deployer",
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with packer auth",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.AuthSectionName: map[string]any{
						"role": "builder",
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with empty components section",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with empty terraform components",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.TerraformComponentType: map[string]any{},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with empty helmfile components",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.HelmfileComponentType: map[string]any{},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with empty packer components",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.PackerComponentType: map[string]any{},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "config with all three component types empty",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.TerraformComponentType: map[string]any{},
					cfg.HelmfileComponentType:  map[string]any{},
					cfg.PackerComponentType:    map[string]any{},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
		{
			name: "comprehensive config with all sections",
			config: map[string]any{
				cfg.NameSectionName: "comprehensive-stack",
				cfg.VarsSectionName: map[string]any{
					"environment": "staging",
					"region":      "us-west-2",
				},
				cfg.SettingsSectionName: map[string]any{
					"spacelift": map[string]any{
						"workspace_enabled": true,
					},
				},
				cfg.EnvSectionName: map[string]any{
					"AWS_REGION": "us-west-2",
				},
				cfg.HooksSectionName: map[string]any{
					"before_all": []any{"echo 'Starting'"},
				},
				cfg.AuthSectionName: map[string]any{
					"default": map[string]any{
						"type": "aws",
					},
				},
				cfg.TerraformSectionName: map[string]any{
					cfg.CommandSectionName:                "terraform",
					cfg.BackendTypeSectionName:            "s3",
					cfg.RemoteStateBackendTypeSectionName: "s3",
					cfg.VarsSectionName: map[string]any{
						"enabled": true,
					},
					cfg.SettingsSectionName: map[string]any{
						"version": "1.5.0",
					},
					cfg.EnvSectionName: map[string]any{
						"TF_LOG": "INFO",
					},
					cfg.BackendSectionName: map[string]any{
						"bucket": "tf-state",
					},
					cfg.RemoteStateBackendSectionName: map[string]any{
						"bucket": "remote-state",
					},
					cfg.ProvidersSectionName: map[string]any{
						"aws": map[string]any{
							"region": "us-west-2",
						},
					},
					cfg.HooksSectionName: map[string]any{
						"before_init": []any{"echo 'Init'"},
					},
					cfg.AuthSectionName: map[string]any{
						"role": "terraform-admin",
					},
				},
				cfg.HelmfileSectionName: map[string]any{
					cfg.CommandSectionName: "helmfile",
					cfg.VarsSectionName: map[string]any{
						"namespace": "staging",
					},
					cfg.SettingsSectionName: map[string]any{
						"version": "0.150.0",
					},
					cfg.EnvSectionName: map[string]any{
						"HELM_DEBUG": "false",
					},
					cfg.AuthSectionName: map[string]any{
						"role": "helm-deployer",
					},
				},
				cfg.PackerSectionName: map[string]any{
					cfg.CommandSectionName: "packer",
					cfg.VarsSectionName: map[string]any{
						"ami_name": "staging-ami",
					},
					cfg.SettingsSectionName: map[string]any{
						"version": "1.8.0",
					},
					cfg.EnvSectionName: map[string]any{
						"PACKER_LOG": "0",
					},
					cfg.AuthSectionName: map[string]any{
						"role": "packer-builder",
					},
				},
				cfg.ComponentsSectionName: map[string]any{
					cfg.TerraformComponentType: map[string]any{},
					cfg.HelmfileComponentType:  map[string]any{},
					cfg.PackerComponentType:    map[string]any{},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				assert.NotNil(t, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessStackConfig(
				atmosConfig,
				"/test/stacks",
				"/test/terraform",
				"/test/helmfile",
				"/test/packer",
				"test-stack.yaml",
				tt.config,
				false,
				false,
				tt.componentTypeFilter,
				map[string]map[string][]string{},
				map[string]map[string]any{},
				false,
			)
			require.NoError(t, err)
			tt.validateResult(t, result)
		})
	}
}

func TestProcessStackConfig_ComponentTypeFilter(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name                string
		componentTypeFilter string
	}{
		{
			name:                "filter terraform components only",
			componentTypeFilter: cfg.TerraformComponentType,
		},
		{
			name:                "filter helmfile components only",
			componentTypeFilter: cfg.HelmfileComponentType,
		},
		{
			name:                "filter packer components only",
			componentTypeFilter: cfg.PackerComponentType,
		},
		{
			name:                "no filter returns all components",
			componentTypeFilter: "",
		},
	}

	config := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformComponentType: map[string]any{},
			cfg.HelmfileComponentType:  map[string]any{},
			cfg.PackerComponentType:    map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessStackConfig(
				atmosConfig,
				"/test/stacks",
				"/test/terraform",
				"/test/helmfile",
				"/test/packer",
				"test-stack.yaml",
				config,
				false,
				false,
				tt.componentTypeFilter,
				map[string]map[string][]string{},
				map[string]map[string]any{},
				false,
			)
			require.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}

func TestProcessStackConfig_CustomComponentTypeFilter(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	// Config with custom component type "script" alongside built-in types.
	config := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformComponentType: map[string]any{
				"vpc": map[string]any{
					cfg.VarsSectionName: map[string]any{"enabled": true},
				},
			},
			"script": map[string]any{
				"deploy-app": map[string]any{
					cfg.VarsSectionName: map[string]any{"app_name": "myapp"},
				},
			},
		},
	}

	tests := []struct {
		name                string
		componentTypeFilter string
		expectTerraform     bool
		expectCustomScript  bool
	}{
		{
			name:                "no filter includes all component types",
			componentTypeFilter: "",
			expectTerraform:     true,
			expectCustomScript:  true,
		},
		{
			name:                "terraform filter excludes custom types",
			componentTypeFilter: cfg.TerraformComponentType,
			expectTerraform:     true,
			expectCustomScript:  false,
		},
		{
			name:                "custom type filter includes only that type",
			componentTypeFilter: "script",
			expectTerraform:     false,
			expectCustomScript:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessStackConfig(
				atmosConfig,
				"/test/stacks",
				"/test/terraform",
				"/test/helmfile",
				"/test/packer",
				"test-stack.yaml",
				config,
				false,
				false,
				tt.componentTypeFilter,
				map[string]map[string][]string{},
				map[string]map[string]any{},
				false,
			)
			require.NoError(t, err)
			require.NotNil(t, result)

			components, ok := result[cfg.ComponentsSectionName].(map[string]any)
			require.True(t, ok, "components section should exist")

			// Check terraform components.
			terraformSection, hasTerraform := components[cfg.TerraformComponentType].(map[string]any)
			if tt.expectTerraform {
				assert.True(t, hasTerraform && len(terraformSection) > 0, "terraform components should be present")
			}

			// Check custom script components.
			scriptSection, hasScript := components["script"].(map[string]any)
			if tt.expectCustomScript {
				assert.True(t, hasScript && len(scriptSection) > 0, "script components should be present")
			} else {
				assert.True(t, !hasScript || len(scriptSection) == 0, "script components should NOT be present when filtered")
			}
		})
	}
}
