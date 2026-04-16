package exec

// terraform_execute_helpers_workspace_test.go contains unit tests for workspace-related
// and TTY-gating helpers extracted from ExecuteTerraform:
//   - runWorkspaceSetup (all early-return paths + wsOpts branch)
//   - checkTTYRequirement (nil-stdin paths)
//   - executeMainTerraformCommand (bare-workspace short-circuit + error propagation)

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

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

// TestRunWorkspaceSetup_OutputSubcommand_HTTPBackend_EarlyReturn verifies that the
// HTTP-backend early-return fires even when SubCommand is "output". The HTTP check
// executes before the wsOpts branch, so this test exercises the HTTP backend
// early-return path, not the wsOpts branch.
func TestRunWorkspaceSetup_OutputSubcommand_HTTPBackend_EarlyReturn(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:           "output",
		ComponentBackendType: "http",
	}
	err := runWorkspaceSetup(&atmosConfig, &info, "/tmp/component")
	assert.NoError(t, err)
}

// TestRunWorkspaceSetup_OutputSubcommand_DryRun_RedirectsToStderr exercises the wsOpts
// branch: when SubCommand is "output" or "show", a WithStdoutOverride(os.Stderr) option
// is set so the "Switched to workspace…" message does not pollute captured stdout.
// DryRun=true bypasses actual terraform execution.
// Must not run in parallel — calls t.Setenv (incompatible with t.Parallel in Go 1.24+).
func TestRunWorkspaceSetup_OutputSubcommand_DryRun_RedirectsToStderr(t *testing.T) {
	// Ensure TF_WORKSPACE is empty so we reach the wsOpts branch and the DryRun-guarded
	// ExecuteShellCommand call instead of the TF_WORKSPACE early-return.
	t.Setenv("TF_WORKSPACE", "")
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "output",
		DryRun:     true,
	}
	err := runWorkspaceSetup(&atmosConfig, &info, t.TempDir())
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

// TestExecuteMainTerraformCommand_Error_Propagates verifies that when the underlying
// ExecuteShellCommand returns an error (non-zero exit from the terraform binary), that
// error is propagated to the caller rather than being swallowed.
//
// Cross-platform approach: uses the test binary itself (os.Executable) with
// _ATMOS_TEST_EXIT_ONE=1 as the "terraform" command.  TestMain in testmain_test.go
// intercepts this env var and calls os.Exit(1) immediately — before the test runner
// starts.  The -test.run argument is irrelevant since TestMain exits before any test
// selection happens, but it is included for documentation clarity.
//
// This test also acts as a contract test that ExecuteShellCommand correctly wraps
// subprocess exit codes in errUtils.ExitCodeError: errors.As must succeed.
func TestExecuteMainTerraformCommand_Error_Propagates(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:       "plan",
		Command:          exePath,
		ComponentEnvList: []string{"_ATMOS_TEST_EXIT_ONE=1"},
		DryRun:           false,
	}

	execErr := executeMainTerraformCommand(&atmosConfig, &info,
		[]string{"-test.run=^$"}, // no test matches → exits 0 normally, but env overrides
		"",                       // component path: current dir
		false,                    // uploadStatusFlag
	)
	require.Error(t, execErr, "executeMainTerraformCommand must propagate non-zero exit from subprocess")

	// Verify the error is wrapped as ExitCodeError (the contract of ExecuteShellCommand).
	var exitCodeErr errUtils.ExitCodeError
	require.True(t,
		errors.As(execErr, &exitCodeErr),
		"error must be wrapped as ExitCodeError, got: %T (%v)", execErr, execErr,
	)
	assert.Equal(t, 1, exitCodeErr.Code, "ExitCodeError.Code must be 1")
}
