package tfmigrate

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

const helperProcessActionEnv = "ATMOS_TFMIGRATE_TEST_HELPER_ACTION"

func TestMain(m *testing.M) {
	switch os.Getenv(helperProcessActionEnv) {
	case "success":
		os.Exit(0)
	case "failure":
		os.Exit(7)
	case "":
		os.Exit(m.Run())
	default:
		os.Exit(2)
	}
}

func TestKindRegistered(t *testing.T) {
	kind, ok := hooks.GetKind("tfmigrate")
	require.True(t, ok)
	assert.Equal(t, tfmigrate.Command, kind.Command)
	assert.Equal(t, hooks.OnFailureFail, kind.OnFailure)
}

func TestAtmosArgs(t *testing.T) {
	args := atmosArgs(&hooks.ExecContext{
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

func TestAtmosArgsForwardsDryRun(t *testing.T) {
	args := atmosArgs(&hooks.ExecContext{
		Hook: &hooks.Hook{},
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
			DryRun:           true,
		},
	}, tfmigrate.ActionApply)
	assert.Equal(t, []string{
		"terraform", "migrate", "apply",
		"vpc",
		"--stack", "plat-ue2-dev",
		"--dry-run",
	}, args)
}

func TestAtmosArgsOmitsDryRunWhenFalse(t *testing.T) {
	args := atmosArgs(&hooks.ExecContext{
		Hook: &hooks.Hook{},
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
		},
	}, tfmigrate.ActionApply)
	assert.NotContains(t, args, "--dry-run")
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
			args := atmosArgs(&hooks.ExecContext{
				Hook: &hooks.Hook{},
				Info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "vpc",
					Stack:            "plat-ue2-dev",
					Identity:         tt.identity,
				},
			}, tfmigrate.ActionPlan)
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

			args := atmosArgs(&hooks.ExecContext{
				Hook: &hooks.Hook{Mode: tfmigrate.ModeDynamic},
				Info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "vpc",
					Stack:            "plat-ue2-dev",
					Identity:         "dev",
				},
			}, action)
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
	args := atmosArgs(&hooks.ExecContext{
		Hook: &hooks.Hook{
			Migration: "migrations/001.hcl",
		},
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
		},
	}, tfmigrate.ActionPlan)
	assert.Equal(t, []string{
		"terraform", "migrate", "plan",
		"vpc",
		"--stack", "plat-ue2-dev",
		"--migration", "migrations/001.hcl",
	}, args)
	assert.NotContains(t, args, "--tfmigrate-config")
}

func TestEngineRunReturnsBeforeSubprocessForInvalidMode(t *testing.T) {
	_, err := (&Engine{}).Run(&hooks.ExecContext{
		Hook:  &hooks.Hook{Mode: "unknown"},
		Event: hooks.BeforeTerraformPlan,
		Info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "plat-ue2-dev"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestEngineRunReturnsBeforeSubprocessForInvalidDynamicEvent(t *testing.T) {
	_, err := (&Engine{}).Run(&hooks.ExecContext{
		Hook:  &hooks.Hook{Mode: tfmigrate.ModeDynamic},
		Event: hooks.AfterTerraformPlan,
		Info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "plat-ue2-dev"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid configuration")
}

func TestEngineRunMissingContextAndEmptyAppendHelpers(t *testing.T) {
	engine := &Engine{}

	_, err := engine.Run(nil)
	require.ErrorIs(t, err, errUtils.ErrNilInput)

	_, err = engine.Run(&hooks.ExecContext{})
	require.ErrorIs(t, err, errUtils.ErrNilInput)

	_, err = engine.Run(&hooks.ExecContext{Hook: &hooks.Hook{}})
	require.ErrorIs(t, err, errUtils.ErrNilInput)

	args := []string{"terraform"}
	assert.Equal(t, args, appendValue(args, ""))
	assert.Equal(t, args, appendFlagValue(args, "--stack", ""))
}

func TestEngineRunExecutesCurrentBinaryWrapper(t *testing.T) {
	useHelperProcess(t, "success")

	output, err := (&Engine{}).Run(&hooks.ExecContext{
		Hook: &hooks.Hook{
			Mode:          tfmigrate.ModeDynamic,
			Migration:     "migrations/001.hcl",
			Config:        ".tfmigrate.hcl",
			BackendConfig: []string{"bucket=state"},
		},
		Event: hooks.BeforeTerraformPlan,
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
			Identity:         cfg.IdentityFlagDisabledValue,
		},
	})

	require.NoError(t, err)
	assert.Nil(t, output)
}

func TestEngineRunWrapsSubprocessFailure(t *testing.T) {
	useHelperProcess(t, "failure")

	// In production, runResolvedHook resolves Hook.OnFailure from the kind's
	// default (hooks.OnFailureFail for tfmigrate, see kind.go's init())
	// before Engine.Run ever sees it - set it explicitly here since this
	// test constructs ExecContext directly, bypassing that resolution step.
	_, err := (&Engine{}).Run(&hooks.ExecContext{
		Hook:  &hooks.Hook{Mode: tfmigrate.ModePlan, OnFailure: hooks.OnFailureFail},
		Event: hooks.BeforeTerraformPlan,
		Info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "plat-ue2-dev"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tfmigrate hook failed")
}

func TestEngineRunHonorsOnFailureWarn(t *testing.T) {
	useHelperProcess(t, "failure")

	// on_failure: warn must swallow the subprocess failure (nil error,
	// matching command_engine.go's ApplyOnFailure semantics for the
	// command/tflint/checkov kinds) instead of always hard-failing.
	output, err := (&Engine{}).Run(&hooks.ExecContext{
		Hook:  &hooks.Hook{Mode: tfmigrate.ModePlan, OnFailure: hooks.OnFailureWarn},
		Event: hooks.BeforeTerraformPlan,
		Info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "plat-ue2-dev"},
	})

	require.NoError(t, err)
	assert.Nil(t, output)
}

func TestEngineRunHonorsOnFailureIgnore(t *testing.T) {
	useHelperProcess(t, "failure")

	output, err := (&Engine{}).Run(&hooks.ExecContext{
		Hook:  &hooks.Hook{Mode: tfmigrate.ModePlan, OnFailure: hooks.OnFailureIgnore},
		Event: hooks.BeforeTerraformPlan,
		Info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "plat-ue2-dev"},
	})

	require.NoError(t, err)
	assert.Nil(t, output)
}

func useHelperProcess(t *testing.T, action string) {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	previousArg0 := os.Args[0]
	os.Args[0] = executable
	t.Setenv(helperProcessActionEnv, action)

	t.Cleanup(func() {
		os.Args[0] = previousArg0
	})
}
