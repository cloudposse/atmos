package tfmigrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

func TestKindRegistered(t *testing.T) {
	kind, ok := hooks.GetKind("tfmigrate")
	require.True(t, ok)
	assert.Equal(t, tfmigrate.Command, kind.Command)
	assert.Equal(t, hooks.OnFailureFail, kind.OnFailure)
}

func TestAtmosArgs(t *testing.T) {
	args, err := atmosArgs(&hooks.ExecContext{
		Hook: &hooks.Hook{
			Migration:     "migrations/001.hcl",
			Config:        ".tfmigrate.hcl",
			BackendConfig: []string{"bucket=state"},
		},
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
			Identity:         "dev",
		},
	}, tfmigrate.ActionApply)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"terraform", "migrate", "apply",
		"vpc",
		"--stack", "plat-ue2-dev",
		"--identity", "dev",
		"--migration", "migrations/001.hcl",
		"--tfmigrate-config", ".tfmigrate.hcl",
		"--backend-config", "bucket=state",
	}, args)
}

func TestAtmosArgsIdentityCases(t *testing.T) {
	tests := []struct {
		name     string
		identity string
		expected []string
	}{
		{
			name: "omits identity when none is resolved",
			expected: []string{
				"terraform", "migrate", "plan",
				"vpc",
				"--stack", "plat-ue2-dev",
			},
		},
		{
			name:     "propagates disabled identity sentinel",
			identity: cfg.IdentityFlagDisabledValue,
			expected: []string{
				"terraform", "migrate", "plan",
				"vpc",
				"--stack", "plat-ue2-dev",
				"--identity", cfg.IdentityFlagDisabledValue,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := atmosArgs(&hooks.ExecContext{
				Hook: &hooks.Hook{},
				Info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "vpc",
					Stack:            "plat-ue2-dev",
					Identity:         tt.identity,
				},
			}, tfmigrate.ActionPlan)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestAtmosArgsDynamicModeActions(t *testing.T) {
	tests := []struct {
		event hooks.HookEvent
		want  string
	}{
		{event: hooks.BeforeTerraformPlan, want: tfmigrate.ActionPlan},
		{event: hooks.BeforeTerraformApply, want: tfmigrate.ActionApply},
		{event: hooks.BeforeTerraformDeploy, want: tfmigrate.ActionApply},
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			action, err := tfmigrate.ActionForMode(tfmigrate.ModeDynamic, string(tt.event))
			require.NoError(t, err)
			assert.Equal(t, tt.want, action)

			args, err := atmosArgs(&hooks.ExecContext{
				Hook: &hooks.Hook{Mode: tfmigrate.ModeDynamic},
				Info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "vpc",
					Stack:            "plat-ue2-dev",
					Identity:         "dev",
				},
			}, action)
			require.NoError(t, err)
			assert.Equal(t, []string{
				"terraform", "migrate", tt.want,
				"vpc",
				"--stack", "plat-ue2-dev",
				"--identity", "dev",
			}, args)
		})
	}
}

func TestAtmosArgs_OmitsOptionalTfmigrateConfig(t *testing.T) {
	args, err := atmosArgs(&hooks.ExecContext{
		Hook: &hooks.Hook{
			Migration: "migrations/001.hcl",
		},
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
		},
	}, tfmigrate.ActionPlan)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"terraform", "migrate", "plan",
		"vpc",
		"--stack", "plat-ue2-dev",
		"--migration", "migrations/001.hcl",
	}, args)
	assert.NotContains(t, args, "--tfmigrate-config")
}
