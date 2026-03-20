package exec

// terraform_execute_helpers_pipeline_test.go contains unit tests for command-pipeline
// and argument-building helpers extracted from ExecuteTerraform:
//   - buildTerraformCommandArgs (unknown subcommand path)
//   - buildWorkspaceSubcommandArgs (delete and select paths)
//   - prepareComponentExecution (early-return error guards)
//   - executeCommandPipeline (TTY error short-circuit via nil stdin)

import (
	"os"
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
