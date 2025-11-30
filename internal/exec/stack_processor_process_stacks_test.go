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
