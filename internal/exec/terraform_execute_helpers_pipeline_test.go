package exec

// terraform_execute_helpers_pipeline_test.go contains unit tests for command-pipeline
// and argument-building helpers extracted from ExecuteTerraform:
//   - buildTerraformCommandArgs (unknown subcommand path)
//   - buildWorkspaceSubcommandArgs (delete and select paths)
//   - prepareComponentExecution (early-return error guards)
//   - executeCommandPipeline (TTY error short-circuit + double-execution regression guard)
//   - runWorkspaceSetup (recovery path when workspace already active)

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
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

// TestExecuteShellCommand_PropagatesEnvToSubprocess is a prerequisite for
// TestRunWorkspaceSetup_RecoveryPath.  It confirms that ExecuteShellCommand
// correctly propagates the ComponentEnvList (env slice) to the spawned subprocess
// so that _ATMOS_TEST_EXIT_ONE=1 actually reaches the process and triggers exit(1).
// Without this guarantee the recovery test would pass vacuously (subprocess never
// exits 1 → no error to recover from → wsErr is nil for the wrong reason).
func TestExecuteShellCommand_PropagatesEnvToSubprocess(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	atmosConfig := schema.AtmosConfiguration{}
	execErr := ExecuteShellCommand(
		atmosConfig, exePath,
		[]string{"-test.run=^$"},           // no test matches → exits 0 normally WITHOUT the env var
		"",                                 // dir: current
		[]string{"_ATMOS_TEST_EXIT_ONE=1"}, // env — should make it exit 1
		false,                              // dryRun
		"",                                 // redirectStdErr
	)
	// The subprocess must have exited 1 (TestMain intercepts _ATMOS_TEST_EXIT_ONE).
	require.Error(t, execErr, "subprocess should have exited 1 when _ATMOS_TEST_EXIT_ONE=1 is propagated")
	var exitErr errUtils.ExitCodeError
	require.True(t, errors.As(execErr, &exitErr), "exit-1 must be wrapped as ExitCodeError, got: %T (%v)", execErr, execErr)
	assert.Equal(t, 1, exitErr.Code, "ExitCodeError.Code must be 1")
}

// TestRunWorkspaceSetup_RecoveryPath verifies that when both "workspace select" and
// "workspace new" fail with exit code 1 but the .terraform/environment file already
// names the target workspace, runWorkspaceSetup logs a warning and returns nil.
// This protects against regressions of the workspace-recovery logic added in this PR.
//
// Cross-platform approach: the test binary (os.Executable) is used as the "terraform"
// command with an env var that triggers immediate exit(1) from TestMain (testmain_test.go).
// This avoids any dependency on platform-specific binaries like "false" (absent on Windows).
// TestExecuteShellCommand_PropagatesEnvToSubprocess (above) verifies that the env is
// actually propagated — ensuring this test cannot pass vacuously.
func TestRunWorkspaceSetup_RecoveryPath(t *testing.T) {
	// Use the test binary itself as the command: it exits 1 immediately when
	// _ATMOS_TEST_EXIT_ONE=1 is set (handled by TestMain in testmain_test.go).
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

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
		// Use the test binary itself as the command: it exits 1 immediately when
		// _ATMOS_TEST_EXIT_ONE=1 is set (handled by TestMain in testmain_test.go).
		Command:          exePath,
		ComponentEnvList: []string{"_ATMOS_TEST_EXIT_ONE=1"},
	}

	// Recovery path: both select and new fail with exit 1, environment file names the
	// workspace → runWorkspaceSetup must return nil (proceed with warning).
	//
	// Capture log output so we can verify the expected recovery warning is emitted.
	var logBuf bytes.Buffer
	log.Default().SetOutput(&logBuf)
	defer log.Default().SetOutput(os.Stderr)

	wsErr := runWorkspaceSetup(&atmosConfig, &info, tmpDir)
	assert.NoError(t, wsErr, "runWorkspaceSetup must succeed when environment file confirms active workspace")

	// Assert the recovery warning was emitted.
	logOutput := logBuf.String()
	assert.True(t,
		strings.Contains(logOutput, "Workspace is already active"),
		"recovery warn log must be emitted when environment file confirms active workspace; got log output: %q", logOutput)
}

// TestRunWorkspaceSetup_NoRecoveryOnMismatchedEnv verifies the negative recovery case:
// when both "workspace select" and "workspace new" fail with exit code 1, and the
// .terraform/environment file names a DIFFERENT workspace than requested, the recovery
// guard must NOT trigger — runWorkspaceSetup must return a non-nil error.
//
// This prevents regressions where recovery triggers too eagerly (e.g., in "staging" but
// requesting "dev" → should fail, not silently continue with the wrong workspace).
//
// Additionally, the warn log ("Workspace is already active…") must NOT be emitted,
// since the recovery path is never entered.
func TestRunWorkspaceSetup_NoRecoveryOnMismatchedEnv(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	tmpDir := t.TempDir()

	// Write "staging" to the environment file but request workspace "dev".
	// isTerraformCurrentWorkspace("dev") must return false → no recovery.
	terraformDir := filepath.Join(tmpDir, ".terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(terraformDir, "environment"),
		[]byte("staging"), // mismatched workspace
		0o600,
	))

	// Redirect log output to a buffer so we can assert no Warn was emitted.
	var logBuf bytes.Buffer
	log.Default().SetOutput(&logBuf)
	defer log.Default().SetOutput(os.Stderr)

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand:         "plan",
		TerraformWorkspace: "dev", // requested workspace differs from active workspace
		Command:            exePath,
		ComponentEnvList:   []string{"_ATMOS_TEST_EXIT_ONE=1"},
	}

	// Both workspace select and new return exit 1, but environment file says "staging"
	// (not "dev") → isTerraformCurrentWorkspace returns false → must return error.
	wsErr := runWorkspaceSetup(&atmosConfig, &info, tmpDir)
	require.Error(t, wsErr, "runWorkspaceSetup must fail when environment file names a different workspace")
	var exitErr errUtils.ExitCodeError
	require.True(t, errors.As(wsErr, &exitErr), "error must be an ExitCodeError, got: %T (%v)", wsErr, wsErr)

	// Assert the recovery warn log was NOT emitted.
	// If it were, it would indicate recovery triggered despite the workspace mismatch.
	logOutput := logBuf.String()
	assert.False(t,
		strings.Contains(logOutput, "Workspace is already active"),
		"recovery warn log must NOT be emitted on mismatch; got log output: %q", logOutput)
}

// ──────────────────────────────────────────────────────────────────────────────
// executeShellCommandWithRetry — per-invocation retry with pattern matching
// ──────────────────────────────────────────────────────────────────────────────

// applyOptsForTest captures the stdout/stderr writers a caller passed via
// WithStdoutCapture / WithStderrCapture so a fake invoke can write to them and
// drive the predicate check inside executeShellCommandWithRetry.
func applyOptsForTest(opts []ShellCommandOption) shellCommandConfig {
	var cfg shellCommandConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// TestExecuteShellCommandWithRetry_NilConfig_RunsOnce verifies that when no retry
// config is present the helper passes through to a single invocation with no capture.
func TestExecuteShellCommandWithRetry_NilConfig_RunsOnce(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{}

	called := 0
	fakeInvoke := func(opts ...ShellCommandOption) error {
		called++
		cfg := applyOptsForTest(opts)
		// Without retry config the helper must not inject capture writers — they
		// would silently swallow the user's terraform output.
		assert.Nil(t, cfg.stdoutCapture, "stdoutCapture must be nil when retry config is absent")
		assert.Nil(t, cfg.stderrCapture, "stderrCapture must be nil when retry config is absent")
		return errors.New("boom")
	}

	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.Error(t, err)
	assert.Equal(t, 1, called, "no retry config = exactly one attempt")
}

// TestExecuteShellCommandWithRetry_NoConditions_RunsOnce verifies that retry config
// without `conditions` is treated as "no retry" (safe default — never blindly retry).
func TestExecuteShellCommandWithRetry_NoConditions_RunsOnce(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentRetrySection: &schema.RetryConfig{MaxAttempts: intPtr(5)},
	}

	called := 0
	fakeInvoke := func(opts ...ShellCommandOption) error {
		called++
		return errors.New("boom")
	}

	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.Error(t, err)
	assert.Equal(t, 1, called, "empty conditions = no retry (safe default)")
}

// TestExecuteShellCommandWithRetry_MatchingError_Retries verifies that when an
// error's captured output matches a `conditions` regex, the command is retried up
// to max_attempts times.
func TestExecuteShellCommandWithRetry_MatchingError_Retries(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(3),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}

	called := 0
	fakeInvoke := func(opts ...ShellCommandOption) error {
		called++
		cfg := applyOptsForTest(opts)
		require.NotNil(t, cfg.stderrCapture, "retry mode must inject stderr capture")
		_, _ = cfg.stderrCapture.Write([]byte("Error: 502 Bad Gateway returned"))
		return errors.New("install failed")
	}

	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.Error(t, err)
	assert.Equal(t, 3, called, "matching error must retry until max_attempts")
}

// TestExecuteShellCommandWithRetry_NonMatchingError_FailsFast verifies that a real
// terraform failure (e.g., plan exit-code-2) is NOT retried when its output does
// not match any condition — protects users from runaway retries on real errors.
func TestExecuteShellCommandWithRetry_NonMatchingError_FailsFast(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(5),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}

	called := 0
	fakeInvoke := func(opts ...ShellCommandOption) error {
		called++
		cfg := applyOptsForTest(opts)
		_, _ = cfg.stderrCapture.Write([]byte("Error: invalid resource argument"))
		return errors.New("plan failed")
	}

	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.Error(t, err)
	assert.Equal(t, 1, called, "non-matching error must fail fast on the first attempt")
}

// TestExecuteShellCommandWithRetry_RecoveryAfterTransient verifies that the helper
// returns nil and stops retrying once an attempt succeeds.
func TestExecuteShellCommandWithRetry_RecoveryAfterTransient(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(5),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}

	called := 0
	fakeInvoke := func(opts ...ShellCommandOption) error {
		called++
		cfg := applyOptsForTest(opts)
		if called < 3 {
			_, _ = cfg.stderrCapture.Write([]byte("502 Bad Gateway"))
			return errors.New("transient")
		}
		// Buffer reset between attempts means the success path writes nothing
		// retryable — predicate returns false on nil err and the loop exits.
		return nil
	}

	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.NoError(t, err)
	assert.Equal(t, 3, called, "must stop retrying as soon as an attempt succeeds")
}

// TestExecuteShellCommandWithRetry_InvalidRegex_FailsFast verifies that a malformed
// regex in `conditions` produces a wrapped ErrInvalidConfig instead of silently
// disabling retry (avoid the foot-gun of silently dropping retry on a typo).
func TestExecuteShellCommandWithRetry_InvalidRegex_FailsFast(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(3),
			Conditions:  []string{"/(/"},
		},
	}

	fakeInvoke := func(opts ...ShellCommandOption) error {
		t.Fatal("invoke must not be called when conditions regex is invalid")
		return nil
	}
	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidConfig),
		"invalid regex must surface as ErrInvalidConfig, got: %v", err)
}

// TestExecuteShellCommandWithRetry_BufferResetBetweenAttempts verifies that
// captured output from a previous attempt does NOT leak into the predicate of a
// later attempt — otherwise a transient match would force retry forever even
// after the error pattern stopped appearing.
func TestExecuteShellCommandWithRetry_BufferResetBetweenAttempts(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		ComponentRetrySection: &schema.RetryConfig{
			MaxAttempts: intPtr(5),
			Conditions:  []string{"/Bad Gateway/"},
		},
	}

	called := 0
	fakeInvoke := func(opts ...ShellCommandOption) error {
		called++
		cfg := applyOptsForTest(opts)
		if called == 1 {
			_, _ = cfg.stderrCapture.Write([]byte("502 Bad Gateway"))
			return errors.New("transient")
		}
		// Second attempt emits a NON-matching error; if the buffer carried over
		// from attempt 1 the predicate would still match and we'd keep retrying.
		_, _ = cfg.stderrCapture.Write([]byte("permission denied"))
		return errors.New("real failure")
	}

	err := executeShellCommandWithRetry(&atmosConfig, &info, "test", fakeInvoke)
	require.Error(t, err)
	assert.Equal(t, 2, called, "buffer must reset so attempt 2's non-matching error stops the loop")
}

// Compile-time guarantee that we are exercising the same `shellCommandConfig`
// the helper writes into. If the field is renamed this test file will fail to
// compile before the regression can ship.
var _ = shellCommandConfig{stdoutCapture: nil, stderrCapture: nil}
