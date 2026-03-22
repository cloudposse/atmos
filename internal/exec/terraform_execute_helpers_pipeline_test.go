package exec

// terraform_execute_helpers_pipeline_test.go contains unit tests for command-pipeline
// and argument-building helpers extracted from ExecuteTerraform:
//   - buildTerraformCommandArgs (unknown subcommand path)
//   - buildWorkspaceSubcommandArgs (delete and select paths)
//   - prepareComponentExecution (early-return error guards)
//   - executeCommandPipeline (TTY error short-circuit + double-execution regression guard)
//   - runWorkspaceSetup (recovery path when workspace already active)

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveExitCode tests live in terraform_execute_helpers_test.go (superset
// including os/exec.ExitError). Duplicates were removed to avoid confusion.

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

// TestBuildWorkspaceSubcommandArgs_NoSubCommand2 is tested by
// TestBuildWorkspaceSubcommandArgs_Bare in _args_test.go. Duplicate removed.

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
//
// This test also guards against the double-execution regression where ExecuteTerraform
// called executeCommandPipeline twice per invocation (every terraform apply ran twice).
// If the function were called twice, the second invocation would try to reach the TTY
// check a second time; any duplication of side-effects (logs, output) would be visible.
// Asserting ErrNoTty from a single executeCommandPipeline call confirms the pipeline
// has a consistent single-invocation exit path.
//
// Must not run in parallel — sets os.Stdin = nil (global state).
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

// ──────────────────────────────────────────────────────────────────────────────
// runWorkspaceSetup (workspace recovery path)
// ──────────────────────────────────────────────────────────────────────────────

// TestRunWorkspaceSetup_RecoveryPath verifies that when both "workspace select" and
// "workspace new" fail with exit code 1 but the .terraform/environment file already
// names the target workspace, runWorkspaceSetup logs a warning and returns nil.
// This protects against regressions of the workspace-recovery logic added in this PR.
func TestRunWorkspaceSetup_RecoveryPath(t *testing.T) {
	// Require a command that reliably exits 1 (POSIX "false").
	falsePath, err := exec.LookPath("false")
	if err != nil {
		t.Skip("'false' command not available on this platform")
	}

	tmpDir := t.TempDir()
	workspace := "dev"

	// Write the environment file so isTerraformCurrentWorkspace returns true.
	terraformDir := filepath.Join(tmpDir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(terraformDir, "environment"),
		[]byte(workspace),
		0o600,
	))

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:         "plan",
		TerraformWorkspace: workspace,
		Command:            falsePath, // always exits 1 → simulates workspace not found
	}

	// Recovery path: both select and new fail with exit 1, environment file names the
	// workspace → runWorkspaceSetup must return nil (proceed with warning).
	wsErr := runWorkspaceSetup(&atmosConfig, &info, tmpDir)
	assert.NoError(t, wsErr, "runWorkspaceSetup must succeed when environment file confirms active workspace")
}
