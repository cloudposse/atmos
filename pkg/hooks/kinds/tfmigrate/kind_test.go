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

func TestAtmosArgsDryRun(t *testing.T) {
	tests := []struct {
		name    string
		dryRun  bool
		wantArg bool
	}{
		{name: "forwards --dry-run when true", dryRun: true, wantArg: true},
		{name: "omits --dry-run when false", dryRun: false, wantArg: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := atmosArgs(&hooks.ExecContext{
				Hook: &hooks.Hook{},
				Info: &schema.ConfigAndStacksInfo{
					ComponentFromArg: "vpc",
					Stack:            "plat-ue2-dev",
					DryRun:           tt.dryRun,
				},
			}, tfmigrate.ActionApply)

			if tt.wantArg {
				assert.Equal(t, []string{
					"terraform", "migrate", "apply",
					"vpc",
					"--stack", "plat-ue2-dev",
					"--dry-run",
				}, args)
				return
			}
			assert.NotContains(t, args, "--dry-run")
		})
	}
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

// TestEngineRunOnFailureBehavior covers on_failure handling for a failing
// subprocess: fail hard-fails (matching runResolvedHook's default resolution
// of Hook.OnFailure to hooks.OnFailureFail for tfmigrate, see kind.go's
// init() - set explicitly here since this test constructs ExecContext
// directly, bypassing that resolution step); warn and ignore must both
// swallow the failure (nil error), matching command_engine.go's
// ApplyOnFailure semantics for the command/tflint/checkov kinds.
func TestEngineRunOnFailureBehavior(t *testing.T) {
	tests := []struct {
		name          string
		onFailure     string
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:          "fail hard-fails on subprocess failure",
			onFailure:     hooks.OnFailureFail,
			wantErr:       true,
			wantErrSubstr: "tfmigrate hook failed",
		},
		{
			name:      "warn swallows the subprocess failure",
			onFailure: hooks.OnFailureWarn,
		},
		{
			name:      "ignore swallows the subprocess failure",
			onFailure: hooks.OnFailureIgnore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useHelperProcess(t, "failure")

			output, err := (&Engine{}).Run(&hooks.ExecContext{
				Hook:  &hooks.Hook{Mode: tfmigrate.ModePlan, OnFailure: tt.onFailure},
				Event: hooks.BeforeTerraformPlan,
				Info:  &schema.ConfigAndStacksInfo{ComponentFromArg: "vpc", Stack: "plat-ue2-dev"},
			})

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubstr)
				return
			}
			require.NoError(t, err)
			assert.Nil(t, output)
		})
	}
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
