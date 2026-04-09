package exec

// terraform_execute_exit_wrapping_test.go is a dedicated contract test for the
// ExitCodeError wrapping guarantee of ExecuteShellCommand.
//
// Contract: whenever the spawned subprocess exits with a non-zero code,
// ExecuteShellCommand must return an error that satisfies errors.As(err, ExitCodeError).
// This guarantee is relied upon by runWorkspaceSetup, executeMainTerraformCommand, and
// any other caller that needs to inspect the subprocess exit code.
//
// Cross-platform approach: uses the test binary (os.Executable) with
// _ATMOS_TEST_EXIT_ONE=1.  TestMain in testmain_test.go intercepts that env var
// and calls os.Exit(1) immediately.

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteShellCommand_ExitOneWrappedAsExitCodeError verifies the core ExitCodeError
// wrapping contract: a subprocess that exits with code 1 must produce an error that can
// be unwrapped to errUtils.ExitCodeError with Code == 1.
//
// This is the canonical standalone reference test for this contract.  The same property
// is exercised indirectly by workspace and pipeline tests that use _ATMOS_TEST_EXIT_ONE,
// but having an explicit dedicated test makes regressions immediately obvious in isolation.
func TestExecuteShellCommand_ExitOneWrappedAsExitCodeError(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	atmosConfig := schema.AtmosConfiguration{}
	execErr := ExecuteShellCommand(
		atmosConfig,
		exePath,
		[]string{"-test.run=^$"},           // no test matches; TestMain exits before any test
		"",                                 // dir: current working directory
		[]string{"_ATMOS_TEST_EXIT_ONE=1"}, // env: makes TestMain call os.Exit(1)
		false,                              // dryRun: false — actually run the subprocess
		"",                                 // redirectStdErr
	)

	// The subprocess must have exited with code 1.
	require.Error(t, execErr, "ExecuteShellCommand must return an error when subprocess exits 1")

	// The error must be (or wrap) an ExitCodeError — callers depend on this contract.
	var exitCodeErr errUtils.ExitCodeError
	require.True(t,
		errors.As(execErr, &exitCodeErr),
		"error must satisfy errors.As(err, ExitCodeError); got %T: %v", execErr, execErr,
	)
	assert.Equal(t, 1, exitCodeErr.Code, "ExitCodeError.Code must equal the subprocess exit code (1)")
}
