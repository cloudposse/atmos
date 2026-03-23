package exec

// terraform_execute_single_invocation_test.go guards against the double-execution
// regression where executeCommandPipeline was called twice per ExecuteTerraform
// invocation (causing every terraform command to run twice).
//
// Strategy: use the test binary (os.Executable) as the "terraform" command.
// TestMain in testmain_test.go writes one byte to _ATMOS_TEST_COUNTER_FILE on
// every invocation, then exits 1 when _ATMOS_TEST_EXIT_ONE=1 is also set.
// After executeCommandPipeline returns, we read the file length: len == 1 means
// exactly one shell invocation occurred.
//
// Why test at executeCommandPipeline level rather than ExecuteTerraform:
//   - ExecuteTerraform requires a full atmos config + real stack files.
//   - executeCommandPipeline is the direct orchestrator of the command pipeline and
//     the function that was double-called in the original regression.
//
// The test uses HTTP backend (ComponentBackendType="http") to skip workspace setup,
// so the counter captures exactly the single main-command invocation via
// executeMainTerraformCommand.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteCommandPipeline_SingleInvocation verifies that executeCommandPipeline
// triggers exactly one shell command invocation, guarding against the double-execution
// regression where the main terraform command ran twice per atmos invocation.
//
// The test binary (os.Executable) acts as the "terraform" command.  On every
// invocation it appends one byte to a temp file (_ATMOS_TEST_COUNTER_FILE), then
// exits 1 (_ATMOS_TEST_EXIT_ONE=1).  After the pipeline call, the file length
// equals the number of times ExecuteShellCommand spawned the subprocess.
//
// Must NOT run in parallel — modifies os.Stdin to nil (global state).
func TestExecuteCommandPipeline_SingleInvocation(t *testing.T) {
	// Set os.Stdin = nil so checkTTYRequirement does not block waiting for a terminal.
	// Using SkipInit + HTTP backend means we reach executeMainTerraformCommand directly.
	origStdin := os.Stdin
	os.Stdin = nil
	t.Cleanup(func() { os.Stdin = origStdin })

	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	// Create a temp file for the invocation counter.
	counterFile := filepath.Join(t.TempDir(), "invocations.txt")

	atmosConfig := schema.AtmosConfiguration{}
	info := schema.ConfigAndStacksInfo{
		SubCommand: "plan",
		// HTTP backend skips workspace setup → only executeMainTerraformCommand calls ExecuteShellCommand.
		ComponentBackendType: "http",
		// SkipInit=true to avoid the init pre-step from triggering a second subprocess.
		SkipInit: true,
		Command:  exePath,
		// Both env vars must be present: counter write THEN exit 1.
		ComponentEnvList: []string{
			"_ATMOS_TEST_COUNTER_FILE=" + counterFile,
			"_ATMOS_TEST_EXIT_ONE=1",
		},
	}
	execCtx := &componentExecContext{
		componentPath: t.TempDir(),
		varFile:       "",
		planFile:      "",
		workingDir:    t.TempDir(),
	}

	// We expect an error because the subprocess exits 1.  The key assertion is
	// the counter file — not the error value itself.
	_ = executeCommandPipeline(&atmosConfig, &info, execCtx)

	// Read the counter file.  Each byte written represents one subprocess invocation.
	data, readErr := os.ReadFile(counterFile)
	if readErr != nil {
		// If the file does not exist the subprocess was never called at all —
		// that is also a regression (missing execution, not double execution).
		require.NoError(t, readErr, "counter file must exist: subprocess was never invoked")
	}
	invocations := len(data)
	assert.Equal(t, 1, invocations,
		"executeCommandPipeline must invoke ExecuteShellCommand exactly once; got %d invocation(s)",
		invocations)
}
