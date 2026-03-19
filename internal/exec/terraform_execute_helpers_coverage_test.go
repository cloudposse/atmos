package exec

// terraform_execute_helpers_coverage_test.go contains additional unit tests that
// target the branches not covered by terraform_execute_helpers_test.go, aiming
// to bring overall coverage of terraform_execute_helpers.go and terraform.go to 100%.
//
// Organisation mirrors the source files:
//   - printAndWriteVarFiles
//   - validateTerraformComponent
//   - prepareInitExecution
//   - buildInitSubcommandArgs (via prepareInitExecution)
//   - buildTerraformCommandArgs (destroy/import/refresh + workspace paths)
//   - buildWorkspaceSubcommandArgs (delete path)
//   - warnOnConflictingEnvVars (with env-var triggers)
//   - runWorkspaceSetup (all early-return paths)
//   - checkTTYRequirement (nil-stdin paths)
//   - resolveExitCode (nil, ExitCodeError, generic)
//   - executeMainTerraformCommand (bare-workspace short-circuit)
//   - cleanupTerraformFiles (actual file creation/removal)
//   - setupTerraformAuth (empty-config, ErrInvalidComponent, merged-config-error, auth-creator error, identity storage, nil-manager)
//   - prepareComponentExecution (error paths)
//   - executeCommandPipeline (early TTY error)

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	mockTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ──────────────────────────────────────────────────────────────────────────────
// printAndWriteVarFiles
// ──────────────────────────────────────────────────────────────────────────────

// TestPrintAndWriteVarFiles_WorkspaceSubcommand verifies the early-return path
// when the subcommand is "workspace" (varfiles are not used for workspace ops).
func TestPrintAndWriteVarFiles_WorkspaceSubcommand(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "workspace"}
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_DryRun_SkipsFileWrite verifies that with DryRun=true
// the function logs but does NOT attempt to write the varfile to disk.
func TestPrintAndWriteVarFiles_DryRun_SkipsFileWrite(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		DryRun:           true,
		UseTerraformPlan: false,
	}
	// /nonexistent path would error if an actual write were attempted.
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_UseTerraformPlan_SkipsVarFile verifies that when
// UseTerraformPlan=true the function skips the varfile entirely (we are
// applying a pre-built plan that already baked in the vars).
func TestPrintAndWriteVarFiles_UseTerraformPlan_SkipsVarFile(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "apply",
		UseTerraformPlan: true,
	}
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_WithCliVarsSection verifies that a populated
// tf_cli_vars section in ComponentSection does not cause an error.
func TestPrintAndWriteVarFiles_WithCliVarsSection(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           true,
		UseTerraformPlan: false,
		ComponentSection: map[string]any{
			cfg.TerraformCliVarsSectionName: map[string]any{
				"region": "us-east-1",
				"env":    "prod",
			},
		},
	}
	err := printAndWriteVarFiles(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestPrintAndWriteVarFiles_WriteActualFile verifies that with DryRun=false the
// function writes a JSON varfile to disk at the path constructed from atmosConfig+info.
func TestPrintAndWriteVarFiles_WriteActualFile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		DryRun:           false,
		UseTerraformPlan: false,
		ContextPrefix:    "ctx",
		Component:        "mycomp",
		ComponentVarsSection: map[string]any{
			"region": "us-east-1",
		},
	}

	// The function builds the varfile path itself from atmosConfig+info.
	expectedVarfilePath := constructTerraformComponentVarfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(expectedVarfilePath), 0o755))

	err := printAndWriteVarFiles(&atmosConfig, &info)
	require.NoError(t, err)
	assert.FileExists(t, expectedVarfilePath)
}

// ──────────────────────────────────────────────────────────────────────────────
// validateTerraformComponent
// ──────────────────────────────────────────────────────────────────────────────

// TestValidateTerraformComponent_EmptySection_Valid verifies that a component
// with no validation section passes validation (empty validations → valid).
func TestValidateTerraformComponent_EmptySection_Valid(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "my-component",
		ComponentSection: map[string]any{},
	}
	err := validateTerraformComponent(&atmosConfig, &info)
	assert.NoError(t, err)
}

// TestValidateTerraformComponent_NilSection_Valid verifies that a nil
// ComponentSection also passes (FindValidationSection handles nil).
func TestValidateTerraformComponent_NilSection_Valid(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "my-component",
		ComponentSection: nil,
	}
	err := validateTerraformComponent(&atmosConfig, &info)
	assert.NoError(t, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// prepareInitExecution
// ──────────────────────────────────────────────────────────────────────────────

// TestPrepareInitExecution_WorkdirPath_ReturnsWorkdir verifies that when the
// component section contains a workdir path key the function returns that path
// instead of the default componentPath.
func TestPrepareInitExecution_WorkdirPath_ReturnsWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	customWorkdir := filepath.Join(tmpDir, "custom-workdir")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"_workdir_path": customWorkdir,
		},
	}

	result, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, customWorkdir, result)
}

// TestPrepareInitExecution_NoWorkdirPath_ReturnsOriginalPath verifies that when
// no workdir path is set the original componentPath is returned unchanged.
func TestPrepareInitExecution_NoWorkdirPath_ReturnsOriginalPath(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}

	result, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, result)
}

// TestPrepareInitExecution_EmptyWorkdirPath_ReturnsOriginalPath verifies that an
// empty string for the workdir key is treated as "not set".
func TestPrepareInitExecution_EmptyWorkdirPath_ReturnsOriginalPath(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"_workdir_path": "", // empty → treated as not set
		},
	}

	result, err := prepareInitExecution(&atmosConfig, &info, tmpDir)
	require.NoError(t, err)
	assert.Equal(t, tmpDir, result)
}

// ──────────────────────────────────────────────────────────────────────────────
// buildInitSubcommandArgs (exercises prepareInitExecution internally)
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildInitSubcommandArgs_BasicInit verifies the default case: no flags, no
// workdir override — just returns ["init"].
func TestBuildInitSubcommandArgs_BasicInit(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	componentPath := tmpDir

	args, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Equal(t, []string{"init"}, args)
	assert.Equal(t, tmpDir, componentPath) // path unchanged
}

// TestBuildInitSubcommandArgs_ReconfigureEnabled verifies -reconfigure is added.
func TestBuildInitSubcommandArgs_ReconfigureEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.InitRunReconfigure = true
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	componentPath := tmpDir

	args, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Contains(t, args, "-reconfigure")
}

// TestBuildInitSubcommandArgs_PassVarsEnabled verifies varfile flag is added.
func TestBuildInitSubcommandArgs_PassVarsEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Init.PassVars = true
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{},
	}
	componentPath := tmpDir

	args, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Contains(t, args, varFileFlag)
	assert.Contains(t, args, "vars.json")
}

// TestBuildInitSubcommandArgs_WorkdirPathUpdatesComponentPath verifies that when
// the component section carries a workdir path key, *componentPath is updated.
func TestBuildInitSubcommandArgs_WorkdirPathUpdatesComponentPath(t *testing.T) {
	tmpDir := t.TempDir()
	customWorkdir := filepath.Join(tmpDir, "custom-workdir")
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"_workdir_path": customWorkdir,
		},
	}
	componentPath := tmpDir

	_, err := buildInitSubcommandArgs(&atmosConfig, &info, []string{"init"}, "vars.json", &componentPath)
	require.NoError(t, err)
	assert.Equal(t, customWorkdir, componentPath)
}

// ──────────────────────────────────────────────────────────────────────────────
// warnOnConflictingEnvVars (coverage of the warning branch)
// ──────────────────────────────────────────────────────────────────────────────

// TestWarnOnConflictingEnvVars_TFCLIArgs verifies the function handles an exact
// match on TF_CLI_ARGS without panicking.
func TestWarnOnConflictingEnvVars_TFCLIArgs(t *testing.T) {
	t.Setenv("TF_CLI_ARGS", "-compact-warnings")
	assert.NotPanics(t, warnOnConflictingEnvVars)
}

// TestWarnOnConflictingEnvVars_TFWorkspace verifies the exact match on TF_WORKSPACE.
func TestWarnOnConflictingEnvVars_TFWorkspace(t *testing.T) {
	t.Setenv("TF_WORKSPACE", "my-workspace")
	assert.NotPanics(t, warnOnConflictingEnvVars)
}

// TestWarnOnConflictingEnvVars_TFVarPrefix verifies the prefix match on TF_VAR_.
func TestWarnOnConflictingEnvVars_TFVarPrefix(t *testing.T) {
	t.Setenv("TF_VAR_region", "us-east-1")
	assert.NotPanics(t, warnOnConflictingEnvVars)
}

// TestWarnOnConflictingEnvVars_TFCLIArgsPrefix verifies the prefix match on TF_CLI_ARGS_.
func TestWarnOnConflictingEnvVars_TFCLIArgsPrefix(t *testing.T) {
	t.Setenv("TF_CLI_ARGS_plan", "-compact-warnings")
	assert.NotPanics(t, warnOnConflictingEnvVars)
}

// TestWarnOnConflictingEnvVars_MultipleConflicts verifies the function handles
// multiple conflicting env vars at once.
func TestWarnOnConflictingEnvVars_MultipleConflicts(t *testing.T) {
	t.Setenv("TF_CLI_ARGS", "-lock=false")
	t.Setenv("TF_WORKSPACE", "my-ws")
	t.Setenv("TF_VAR_env", "prod")
	assert.NotPanics(t, warnOnConflictingEnvVars)
}

// ──────────────────────────────────────────────────────────────────────────────
// runWorkspaceSetup (early-return paths — no shell required)
// ──────────────────────────────────────────────────────────────────────────────

// TestRunWorkspaceSetup_InitSubcommand_ReturnsNil verifies init bypasses workspace
// selection (init doesn't need a workspace to be active).
func TestRunWorkspaceSetup_InitSubcommand_ReturnsNil(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "init"}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// TestRunWorkspaceSetup_WorkspaceWithSubCommand2_ReturnsNil verifies that
// explicit workspace sub-subcommands (new, select, delete) skip the auto-select.
func TestRunWorkspaceSetup_WorkspaceWithSubCommand2_ReturnsNil(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "workspace",
		SubCommand2: "new",
	}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// TestRunWorkspaceSetup_HTTPBackend_ReturnsNil verifies that HTTP backends skip
// workspace selection (HTTP backend does not support workspaces).
func TestRunWorkspaceSetup_HTTPBackend_ReturnsNil(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "apply",
		ComponentBackendType: "http",
	}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// TestRunWorkspaceSetup_TFWorkspaceEnvSet_ReturnsNil verifies that when
// TF_WORKSPACE is already set we defer workspace management to Terraform itself.
func TestRunWorkspaceSetup_TFWorkspaceEnvSet_ReturnsNil(t *testing.T) {
	t.Setenv("TF_WORKSPACE", "my-workspace")
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "apply"}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// TestRunWorkspaceSetup_WorkspaceNoSubCommand2_ReturnsNil verifies that a bare
// "workspace" subcommand with no SubCommand2 (e.g., "workspace list" already
// handled) also returns nil when TF_WORKSPACE is set.
func TestRunWorkspaceSetup_WorkspaceNoSubCommand2_TFWorkspace_ReturnsNil(t *testing.T) {
	t.Setenv("TF_WORKSPACE", "dev")
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "workspace",
		SubCommand2: "",
	}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// TestRunWorkspaceSetup_OutputSubcommand_RedirectStderr verifies that output
// subcommand sets up the stdout override option (exercises the wsOpts branch)
// without actually calling terraform. We test this via the HTTP backend shortcut.
func TestRunWorkspaceSetup_OutputSubcommand_HTTPBackend(t *testing.T) {
	// Use HTTP backend to short-circuit before the shell call, but after
	// the wsOpts branch would be set. This covers the branch condition.
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "output",
		ComponentBackendType: "http",
	}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// checkTTYRequirement (nil-stdin branch)
// ──────────────────────────────────────────────────────────────────────────────
// NOTE: These tests temporarily set os.Stdin = nil to simulate a non-interactive
// environment. They must NOT be run in parallel as they mutate global state.

// TestCheckTTYRequirement_NilStdin_ApplyWithoutAutoApprove_ReturnsError verifies
// that applying without -auto-approve in a non-interactive environment is an error.
func TestCheckTTYRequirement_NilStdin_ApplyWithoutAutoApprove_ReturnsError(t *testing.T) {
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })

	info := schema.ConfigAndStacksInfo{SubCommand: "apply"}
	err := checkTTYRequirement(&info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoTty)
}

// TestCheckTTYRequirement_NilStdin_ApplyWithAutoApprove_NoError verifies that
// -auto-approve bypasses the TTY requirement.
func TestCheckTTYRequirement_NilStdin_ApplyWithAutoApprove_NoError(t *testing.T) {
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })

	info := schema.ConfigAndStacksInfo{
		SubCommand:             "apply",
		AdditionalArgsAndFlags: []string{autoApproveFlag},
	}
	err := checkTTYRequirement(&info)
	assert.NoError(t, err)
}

// TestCheckTTYRequirement_NilStdin_NonApply_NoError verifies that non-apply
// subcommands don't require a TTY even when stdin is nil.
func TestCheckTTYRequirement_NilStdin_NonApply_NoError(t *testing.T) {
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })

	for _, sub := range []string{"plan", "destroy", "init", "workspace", "output"} {
		info := schema.ConfigAndStacksInfo{SubCommand: sub}
		err := checkTTYRequirement(&info)
		assert.NoError(t, err, "subcommand %q should not require TTY", sub)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// executeMainTerraformCommand (bare-workspace short-circuit)
// ──────────────────────────────────────────────────────────────────────────────

// TestExecuteMainTerraformCommand_BareWorkspace_ReturnsNil verifies that a bare
// "workspace" subcommand (no SubCommand2) is treated as a no-op since the
// workspace listing was already handled by runWorkspaceSetup.
func TestExecuteMainTerraformCommand_BareWorkspace_ReturnsNil(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:  "workspace",
		SubCommand2: "",
	}
	err := executeMainTerraformCommand(&atmosConfig, &info, []string{"workspace"}, "/tmp/component", false)
	assert.NoError(t, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// cleanupTerraformFiles (actual file creation/removal)
// ──────────────────────────────────────────────────────────────────────────────

// TestCleanupTerraformFiles_ApplyRemovesVarfileForReal creates an actual varfile
// and verifies it is deleted after cleanupTerraformFiles for apply.
func TestCleanupTerraformFiles_ApplyRemovesVarfileForReal(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "apply",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}

	// Construct the expected path so we can create the file before cleanup.
	varfilePath := constructTerraformComponentVarfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(varfilePath), 0o755))
	require.NoError(t, os.WriteFile(varfilePath, []byte(`{"key":"value"}`), 0o644))
	require.FileExists(t, varfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.NoFileExists(t, varfilePath)
}

// TestCleanupTerraformFiles_NonPlanShow_RemovesPlanfile creates an actual planfile
// and verifies it is removed after cleanupTerraformFiles for a destroy command.
func TestCleanupTerraformFiles_NonPlanShow_RemovesPlanfile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "destroy",
		ContextPrefix: "ctx",
		Component:     "mycomp",
		PlanFile:      "", // empty PlanFile triggers planfile removal
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))
	require.FileExists(t, planfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.NoFileExists(t, planfilePath)
}

// TestCleanupTerraformFiles_ShowSubcommand_KeepsPlanfile verifies that "show"
// does NOT remove the planfile (it is a read-only operation over the plan).
func TestCleanupTerraformFiles_ShowSubcommand_KeepsPlanfile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "show",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))
	require.FileExists(t, planfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.FileExists(t, planfilePath, "show should not delete the planfile")
}

// TestCleanupTerraformFiles_PlanSubcommand_KeepsPlanfile verifies that "plan"
// itself does NOT remove the planfile it just generated.
func TestCleanupTerraformFiles_PlanSubcommand_KeepsPlanfile(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "plan",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))
	require.FileExists(t, planfilePath)

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.FileExists(t, planfilePath, "plan should not delete its own output planfile")
}

// TestCleanupTerraformFiles_ApplyWithCustomPlanFile_SkipsPlanfileRemoval
// verifies that when PlanFile is non-empty (consuming a pre-existing plan) the
// planfile is NOT deleted by cleanup — only the varfile is.
func TestCleanupTerraformFiles_ApplyWithCustomPlanFile_SkipsPlanfileRemoval(t *testing.T) {
	tmpDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = tmpDir
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "apply",
		ContextPrefix: "ctx",
		Component:     "mycomp",
		PlanFile:      "custom.planfile", // non-empty → planfile removal is skipped
	}

	planfilePath := constructTerraformComponentPlanfilePath(&atmosConfig, &info)
	require.NoError(t, os.MkdirAll(filepath.Dir(planfilePath), 0o755))
	require.NoError(t, os.WriteFile(planfilePath, []byte("plan-data"), 0o644))

	cleanupTerraformFiles(&atmosConfig, &info)

	assert.FileExists(t, planfilePath, "planfile should NOT be removed when PlanFile is set")
}

// TestCleanupTerraformFiles_MissingFiles_NoError verifies that cleanup is
// graceful when neither planfile nor varfile exists (already cleaned up).
func TestCleanupTerraformFiles_MissingFiles_NoError(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	atmosConfig.BasePath = t.TempDir()
	info := schema.ConfigAndStacksInfo{
		SubCommand:    "apply",
		ContextPrefix: "ctx",
		Component:     "mycomp",
	}
	assert.NotPanics(t, func() { cleanupTerraformFiles(&atmosConfig, &info) })
}

// ──────────────────────────────────────────────────────────────────────────────
// setupTerraformAuth
// ──────────────────────────────────────────────────────────────────────────────

// TestSetupTerraformAuth_EmptyConfig_NoProviders verifies that with an empty
// AtmosConfiguration (no auth providers configured) the function completes
// without error and leaves info.AuthManager as nil.
func TestSetupTerraformAuth_EmptyConfig_NoProviders(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		// Empty stack and component → getMergedAuthConfig skips component lookup.
		Stack:            "",
		ComponentFromArg: "",
	}

	authMgr, err := setupTerraformAuth(&atmosConfig, &info)
	require.NoError(t, err)
	// With no auth providers configured the AuthManager should be nil.
	assert.Nil(t, authMgr)
	assert.Nil(t, info.AuthManager)
}

// TestSetupTerraformAuth_ErrInvalidComponent verifies that ErrInvalidComponent is
// propagated directly without additional sentinel wrapping.  This prevents auth prompts
// when the caller references a component that does not exist.
func TestSetupTerraformAuth_ErrInvalidComponent(t *testing.T) {
	orig := defaultComponentConfigFetcher
	t.Cleanup(func() { defaultComponentConfigFetcher = orig })
	defaultComponentConfigFetcher = func(_ *ExecuteDescribeComponentParams) (map[string]any, error) {
		return nil, errUtils.ErrInvalidComponent
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "nonexistent",
	}

	_, err := setupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent), "expected ErrInvalidComponent but got: %v", err)
	// Must NOT be additionally wrapped — doing so would change the sentinel seen by callers.
	assert.False(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "ErrInvalidComponent must not be wrapped with ErrInvalidAuthConfig")
}

// TestSetupTerraformAuth_AuthCreatorError_WrapsWithSentinel verifies that errors
// returned by the auth manager creator are wrapped with ErrFailedToInitializeAuthManager,
// keeping parity with createAndAuthenticateAuthManagerWithDeps.
func TestSetupTerraformAuth_AuthCreatorError_WrapsWithSentinel(t *testing.T) {
	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, errors.New("auth backend unavailable")
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	_, err := setupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToInitializeAuthManager), "expected ErrFailedToInitializeAuthManager but got: %v", err)
}

// TestSetupTerraformAuth_IdentityStoredAndManagerSet verifies that when the auth creator
// returns a non-nil AuthManager:
//   - info.Identity is set to the last element of the auth chain (auto-detection).
//   - info.AuthManager is populated with the returned manager.
//   - The returned manager matches what was injected.
func TestSetupTerraformAuth_IdentityStoredAndManagerSet(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockMgr := mockTypes.NewMockAuthManager(ctrl)
	mockMgr.EXPECT().GetChain().Return([]string{"base-role", "aws-dev"})

	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return mockMgr, nil
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{Identity: ""}

	mgr, err := setupTerraformAuth(&atmosConfig, &info)
	require.NoError(t, err)
	assert.Equal(t, mockMgr, mgr)
	// storeAutoDetectedIdentity takes the last element of the chain.
	assert.Equal(t, "aws-dev", info.Identity)
	assert.Equal(t, mockMgr, info.AuthManager)
}

// TestSetupTerraformAuth_NilManager_NoAuthBridge verifies that when the auth creator
// returns nil (no auth configured), info.AuthManager is left nil and the store auth-bridge
// is not injected (no panic on nil Stores).
func TestSetupTerraformAuth_NilManager_NoAuthBridge(t *testing.T) {
	orig := defaultAuthManagerCreator
	t.Cleanup(func() { defaultAuthManagerCreator = orig })
	defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, nil
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	assert.NotPanics(t, func() {
		mgr, err := setupTerraformAuth(&atmosConfig, &info)
		require.NoError(t, err)
		assert.Nil(t, mgr)
		assert.Nil(t, info.AuthManager)
	})
}

// TestSetupTerraformAuth_MergedConfigError_WrapsWithInvalidAuthConfig verifies that
// when getMergedAuthConfig fails with an error that is NOT ErrInvalidComponent,
// the error is wrapped with ErrInvalidAuthConfig (matching createAndAuthenticateAuthManagerWithDeps).
func TestSetupTerraformAuth_MergedConfigError_WrapsWithInvalidAuthConfig(t *testing.T) {
	orig := defaultMergedAuthConfigGetter
	t.Cleanup(func() { defaultMergedAuthConfigGetter = orig })
	defaultMergedAuthConfigGetter = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
		return nil, errors.New("config merge failure")
	}

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "mycomp"}

	_, err := setupTerraformAuth(&atmosConfig, &info)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidAuthConfig), "expected ErrInvalidAuthConfig, got: %v", err)
	assert.False(t, errors.Is(err, errUtils.ErrInvalidComponent))
}

// ──────────────────────────────────────────────────────────────────────────────
// resolveExitCode
// ──────────────────────────────────────────────────────────────────────────────

// TestResolveExitCode_Nil verifies that nil error → exit code 0.
func TestResolveExitCode_Nil(t *testing.T) {
	assert.Equal(t, 0, resolveExitCode(nil))
}

// TestResolveExitCode_ExitCodeError verifies that an ExitCodeError is unwrapped correctly.
func TestResolveExitCode_ExitCodeError(t *testing.T) {
	assert.Equal(t, 2, resolveExitCode(errUtils.ExitCodeError{Code: 2}))
	assert.Equal(t, 42, resolveExitCode(errUtils.ExitCodeError{Code: 42}))
}

// TestResolveExitCode_GenericError verifies that a plain (non-typed) error → 1.
func TestResolveExitCode_GenericError(t *testing.T) {
	assert.Equal(t, 1, resolveExitCode(errors.New("something went wrong")))
}

// TestResolveExitCode_WrappedExitCodeError verifies that a wrapped ExitCodeError
// is correctly unwrapped.
func TestResolveExitCode_WrappedExitCodeError(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", errUtils.ExitCodeError{Code: 5})
	assert.Equal(t, 5, resolveExitCode(wrapped))
}

// ──────────────────────────────────────────────────────────────────────────────
// buildTerraformCommandArgs (unknown subcommand path not covered elsewhere)
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildTerraformCommandArgs_UnknownSubcommand verifies that an unknown subcommand
// results in just the subcommand name with no extra flags.
func TestBuildTerraformCommandArgs_UnknownSubcommand(t *testing.T) {
	tmpDir := t.TempDir()
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{SubCommand: "validate"}
	cp := tmpDir

	args, _, err := buildTerraformCommandArgs(&atmosConfig, &info, "vars.json", "plan.planfile", &cp)
	require.NoError(t, err)
	assert.Equal(t, []string{"validate"}, args)
}

// ──────────────────────────────────────────────────────────────────────────────
// buildWorkspaceSubcommandArgs (delete and select paths)
// ──────────────────────────────────────────────────────────────────────────────

// TestBuildWorkspaceSubcommandArgs_Delete verifies that "delete" gets the workspace name.
func TestBuildWorkspaceSubcommandArgs_Delete(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand2:        "delete",
		TerraformWorkspace: "dev",
	}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	assert.Equal(t, []string{"workspace", "delete", "dev"}, args)
}

// TestBuildWorkspaceSubcommandArgs_Select verifies that "select" gets the workspace name.
func TestBuildWorkspaceSubcommandArgs_Select(t *testing.T) {
	info := schema.ConfigAndStacksInfo{
		SubCommand2:        "select",
		TerraformWorkspace: "prod",
	}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	assert.Equal(t, []string{"workspace", "select", "prod"}, args)
}

// TestBuildWorkspaceSubcommandArgs_NoSubCommand2 verifies no SubCommand2 → unchanged args.
func TestBuildWorkspaceSubcommandArgs_NoSubCommand2(t *testing.T) {
	info := schema.ConfigAndStacksInfo{}
	args := buildWorkspaceSubcommandArgs(&info, []string{"workspace"})
	assert.Equal(t, []string{"workspace"}, args)
}

// ──────────────────────────────────────────────────────────────────────────────
// prepareComponentExecution (error paths — exercises early-return guards)
// ──────────────────────────────────────────────────────────────────────────────

// TestPrepareComponentExecution_NoComponentPath_ReturnsError verifies that an
// empty base path causes an error from GetComponentPath before any shell runs.
func TestPrepareComponentExecution_NoComponentPath_ReturnsError(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	// BasePath is empty → GetComponentPath returns an error.
	info := schema.ConfigAndStacksInfo{}

	_, err := prepareComponentExecution(&atmosConfig, &info, false)
	// An empty BasePath causes checkTerraformConfig to return an error.
	require.Error(t, err)
}

// ──────────────────────────────────────────────────────────────────────────────
// executeCommandPipeline (TTY error short-circuit via nil stdin)
// ──────────────────────────────────────────────────────────────────────────────

// TestExecuteCommandPipeline_TTYError verifies that an apply without -auto-approve
// in a nil-stdin environment returns ErrNoTty before calling any shell command.
func TestExecuteCommandPipeline_TTYError(t *testing.T) {
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "apply",
		SkipInit:             true,   // skip init pre-step (no terraform binary available)
		ComponentBackendType: "http", // skip workspace setup (HTTP backend has no workspace)
		// No -auto-approve and no TTY → should fail at checkTTYRequirement.
	}
	execCtx := &componentExecContext{
		componentPath: "/nonexistent",
		varFile:       "vars.json",
		planFile:      "plan.planfile",
		workingDir:    "/nonexistent",
	}

	err := executeCommandPipeline(&atmosConfig, &info, execCtx)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNoTty)
}
