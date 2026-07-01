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
		{
			name: "invalid custom component map type",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					"script": map[string]any{
						"deploy-app": "invalid-not-a-map",
					},
				},
			},
			expectedError: errUtils.ErrInvalidComponentMapType,
		},
		{
			name: "invalid ansible section type",
			config: map[string]any{
				cfg.AnsibleSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidAnsibleSection,
		},
		{
			name: "invalid components.ansible type",
			config: map[string]any{
				cfg.ComponentsSectionName: map[string]any{
					cfg.AnsibleComponentType: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidComponentsAnsible,
		},
		{
			name: "invalid dependencies section type",
			config: map[string]any{
				cfg.DependenciesSectionName: "invalid-not-a-map",
			},
			expectedError: errUtils.ErrInvalidDependenciesSection,
		},
		{
			name: "invalid terraform dependencies type",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.DependenciesSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidTerraformDependencies,
		},
		{
			name: "invalid helmfile dependencies type",
			config: map[string]any{
				cfg.HelmfileSectionName: map[string]any{
					cfg.DependenciesSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidHelmfileDependencies,
		},
		{
			name: "invalid packer dependencies type",
			config: map[string]any{
				cfg.PackerSectionName: map[string]any{
					cfg.DependenciesSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidPackerDependencies,
		},
		{
			name: "invalid ansible dependencies type",
			config: map[string]any{
				cfg.AnsibleSectionName: map[string]any{
					cfg.DependenciesSectionName: "invalid",
				},
			},
			expectedError: errUtils.ErrInvalidAnsibleDependencies,
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
				"/test/ansible",
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
			// DRY pattern: a hook defined once at top-level `terraform.hooks`
			// must be inherited by a component that declares no hooks of its own.
			name: "terraform hooks inherited by component with no own hooks",
			config: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					cfg.HooksSectionName: map[string]any{
						"policy": map[string]any{
							"events": []any{"before-terraform-plan"},
							"kind":   "checkov",
						},
					},
				},
				cfg.ComponentsSectionName: map[string]any{
					cfg.TerraformComponentType: map[string]any{
						"vpc": map[string]any{
							cfg.VarsSectionName: map[string]any{"name": "vpc"},
						},
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				hooks := componentHooks(t, result, "vpc")
				policy, ok := hooks["policy"].(map[string]any)
				require.True(t, ok, "component must inherit the top-level terraform.hooks 'policy' hook, got: %v", hooks)
				assert.Equal(t, "checkov", policy["kind"])
				assert.Equal(t, []any{"before-terraform-plan"}, policy["events"])
			},
		},
		{
			// A hook in the global top-level `hooks:` section is likewise
			// inherited by every terraform component.
			name: "global hooks inherited by component with no own hooks",
			config: map[string]any{
				cfg.HooksSectionName: map[string]any{
					"cost": map[string]any{
						"events": []any{"after-terraform-plan"},
						"kind":   "infracost",
					},
				},
				cfg.ComponentsSectionName: map[string]any{
					cfg.TerraformComponentType: map[string]any{
						"vpc": map[string]any{
							cfg.VarsSectionName: map[string]any{"name": "vpc"},
						},
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				hooks := componentHooks(t, result, "vpc")
				cost, ok := hooks["cost"].(map[string]any)
				require.True(t, ok, "component must inherit the global 'cost' hook, got: %v", hooks)
				assert.Equal(t, "infracost", cost["kind"])
				assert.Equal(t, []any{"after-terraform-plan"}, cost["events"])
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
		{
			name: "config with kubernetes section and component",
			config: map[string]any{
				cfg.KubernetesSectionName: map[string]any{
					cfg.CommandSectionName: "kubectl",
					cfg.VarsSectionName: map[string]any{
						"namespace": "default",
					},
					cfg.SettingsSectionName: map[string]any{
						"enabled": true,
					},
					cfg.EnvSectionName: map[string]any{
						"KUBECONFIG": "/path/to/config",
					},
					cfg.AuthSectionName: map[string]any{
						"role": "deployer",
					},
					cfg.DependenciesSectionName: map[string]any{
						"file": []any{"dep"},
					},
					cfg.SourceSectionName: map[string]any{
						"uri": "github.com/example/repo",
					},
					cfg.ProvisionSectionName: map[string]any{
						"workdir": ".",
					},
					cfg.ProviderSectionName:  "kustomize",
					cfg.PathsSectionName:     []any{"base"},
					cfg.ManifestsSectionName: map[string]any{"deployment": "d.yaml"},
					cfg.RenderSectionName:    map[string]any{"engine": "kustomize"},
				},
				cfg.ComponentsSectionName: map[string]any{
					cfg.KubernetesComponentType: map[string]any{
						"app": map[string]any{
							cfg.VarsSectionName: map[string]any{"replicas": 3},
						},
					},
				},
			},
			validateResult: func(t *testing.T, result map[string]any) {
				components, ok := result[cfg.ComponentsSectionName].(map[string]any)
				require.True(t, ok, "result must contain a components section")
				kubernetes, ok := components[cfg.KubernetesComponentType].(map[string]any)
				require.True(t, ok, "result must contain kubernetes components")
				app, ok := kubernetes["app"].(map[string]any)
				require.True(t, ok, "kubernetes component 'app' must exist")
				// Stack-global kubernetes provider/paths/manifests/render flow into the component.
				assert.Equal(t, "kustomize", app[cfg.ProviderSectionName])
				assert.Equal(t, []any{"base"}, app[cfg.PathsSectionName])
				assert.Equal(t, map[string]any{"deployment": "d.yaml"}, app[cfg.ManifestsSectionName])
				assert.Equal(t, map[string]any{"engine": "kustomize"}, app[cfg.RenderSectionName])
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
				"/test/ansible",
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

// TestProcessStackConfig_KubernetesErrorPaths covers the stack-global kubernetes section
// type-validation branches: command/vars/settings/env/auth/dependencies/source/provision/
// provider/render must each be the expected type or ProcessStackConfig returns a precise
// error.
func TestProcessStackConfig_KubernetesErrorPaths(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	tests := []struct {
		name          string
		k8sSection    map[string]any
		expectedError error
	}{
		{"invalid kubernetes section type", nil, errUtils.ErrInvalidConfig},
		{"invalid command type", map[string]any{cfg.CommandSectionName: 123}, errUtils.ErrInvalidComponentCommand},
		{"invalid vars type", map[string]any{cfg.VarsSectionName: "x"}, errUtils.ErrInvalidVarsSection},
		{"invalid hooks type", map[string]any{cfg.HooksSectionName: "x"}, errUtils.ErrInvalidHooksSection},
		{"invalid generate type", map[string]any{cfg.GenerateSectionName: "x"}, errUtils.ErrInvalidGenerateSection},
		{"invalid settings type", map[string]any{cfg.SettingsSectionName: "x"}, errUtils.ErrInvalidSettingsSection},
		{"invalid env type", map[string]any{cfg.EnvSectionName: "x"}, errUtils.ErrInvalidEnvSection},
		{"invalid auth type", map[string]any{cfg.AuthSectionName: "x"}, errUtils.ErrInvalidAuthSection},
		{"invalid dependencies type", map[string]any{cfg.DependenciesSectionName: "x"}, errUtils.ErrInvalidDependenciesSection},
		{"invalid source type", map[string]any{cfg.SourceSectionName: "x"}, errUtils.ErrInvalidComponentSource},
		{"invalid provision type", map[string]any{cfg.ProvisionSectionName: "x"}, errUtils.ErrInvalidComponentProvision},
		{"invalid provider type", map[string]any{cfg.ProviderSectionName: 123}, errUtils.ErrInvalidConfig},
		{"invalid render type", map[string]any{cfg.RenderSectionName: "x"}, errUtils.ErrInvalidConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config map[string]any
			if tt.k8sSection == nil {
				// Top-level kubernetes section is not a map.
				config = map[string]any{cfg.KubernetesSectionName: "invalid-not-a-map"}
			} else {
				config = map[string]any{cfg.KubernetesSectionName: tt.k8sSection}
			}

			_, err := ProcessStackConfig(
				atmosConfig,
				"/test/stacks",
				"/test/terraform",
				"/test/helmfile",
				"/test/packer",
				"/test/ansible",
				"test-stack.yaml",
				config,
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
				"/test/ansible",
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
				"/test/ansible",
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
				// Verify metadata injection.
				deployApp, ok := scriptSection["deploy-app"].(map[string]any)
				require.True(t, ok, "deploy-app component should exist")
				assert.Equal(t, "deploy-app", deployApp["component"], "component name should be injected")
				assert.Equal(t, "script", deployApp[cfg.ComponentTypeSectionName], "component_type should be injected")
				// Verify vars are present.
				vars, ok := deployApp[cfg.VarsSectionName].(map[string]any)
				require.True(t, ok, "vars should be merged")
				assert.Equal(t, "myapp", vars["app_name"], "component vars should be preserved")
			} else {
				assert.True(t, !hasScript || len(scriptSection) == 0, "script components should NOT be present when filtered")
			}
		})
	}
}

// componentHooks extracts the merged hooks section for a terraform component
// from a ProcessStackConfig result. It fails the test if the component or its
// hooks section is missing, so inheritance assertions read cleanly.
func componentHooks(t *testing.T, result map[string]any, component string) map[string]any {
	t.Helper()
	components, ok := result[cfg.ComponentsSectionName].(map[string]any)
	require.True(t, ok, "result must contain a components section")
	terraform, ok := components[cfg.TerraformComponentType].(map[string]any)
	require.True(t, ok, "result must contain terraform components")
	comp, ok := terraform[component].(map[string]any)
	require.True(t, ok, "terraform component %q must exist", component)
	hooks, ok := comp[cfg.HooksSectionName].(map[string]any)
	require.True(t, ok, "component %q must have a hooks section, got: %v", component, comp[cfg.HooksSectionName])
	return hooks
}

// TestProcessStackConfig_HooksWrongScopeNotInherited locks in the scope
// distinction that tripped up the original report: hooks belong at top-level
// `terraform.hooks` (or global `hooks:`), NOT under `components.terraform.hooks`.
// A `hooks` key directly under `components.terraform` is a sibling of the
// component names, so it is treated as a component literally named "hooks" and
// is NOT inherited as hook defaults by real components.
func TestProcessStackConfig_HooksWrongScopeNotInherited(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}

	config := map[string]any{
		cfg.ComponentsSectionName: map[string]any{
			cfg.TerraformComponentType: map[string]any{
				// Misplaced: this is the WRONG scope for shared hook defaults.
				cfg.HooksSectionName: map[string]any{
					"policy": map[string]any{
						"events": []any{"before-terraform-plan"},
						"kind":   "checkov",
					},
				},
				"vpc": map[string]any{
					cfg.VarsSectionName: map[string]any{"name": "vpc"},
				},
			},
		},
	}

	result, err := ProcessStackConfig(
		atmosConfig,
		"/test/stacks",
		"/test/terraform",
		"/test/helmfile",
		"/test/packer",
		"/test/ansible",
		"test-stack.yaml",
		config,
		false,
		false,
		"",
		map[string]map[string][]string{},
		map[string]map[string]any{},
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	components, ok := result[cfg.ComponentsSectionName].(map[string]any)
	require.True(t, ok, "result must contain a components section")
	terraform, ok := components[cfg.TerraformComponentType].(map[string]any)
	require.True(t, ok, "result must contain terraform components")

	// The real component must NOT inherit the misplaced hook.
	vpc, ok := terraform["vpc"].(map[string]any)
	require.True(t, ok, "vpc component must exist")
	if hooks, ok := vpc[cfg.HooksSectionName].(map[string]any); ok {
		_, leaked := hooks["policy"]
		assert.False(t, leaked, "component must NOT inherit a hook placed under components.terraform.hooks (wrong scope)")
	}
}
