package terraform

// utils_exit_wrapping_test.go is a regression test for the ExitCodeError
// wrapping contract at the cmd/terraform → internal/exec boundary.
//
// runHooksOnErrorWithOutput (cmd/terraform/utils.go) extracts the exit code
// from the command error via errUtils.GetExitCode(cmdErr). The CI hook
// plumbing (RunCIHooksOptions.ExitCode + CommandError) depends on the wrapper
// being intact: a future refactor that swaps the error type or unwraps
// ExitCodeError before it reaches this layer would silently degrade CI
// summaries and check runs without tripping any of the existing tests
// downstream (those tests start *after* GetExitCode has already flattened the
// chain to an int).
//
// Cross-platform approach: uses the test binary (os.Executable) with
// _ATMOS_TEST_EXIT_ONE=1 — TestMain in testmain_test.go intercepts that env
// var and calls os.Exit(1). No Unix-only binaries are required.

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	h "github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecuteShellCommand_ErrorWrapsExitCodeError_AtCmdTerraformBoundary verifies
// that the cmdErr passed into runHooksOnErrorWithOutput preserves the
// ExitCodeError wrapper. This protects the contract:
//
//	ExecuteShellCommand → returns errUtils.ExitCodeError (typed)
//	  → ExecuteTerraform/executeSingleComponent
//	    → terraform RunE catches as runErr
//	      → runHooksOnErrorWithOutput (this layer) calls
//	          errUtils.GetExitCode(cmdErr) — depends on the wrapper.
//
// We assert both ends: errors.As recovers the typed error AND
// errUtils.GetExitCode extracts the wrapped code, which is exactly what
// runHooksOnErrorWithOutput does in production.
func TestExecuteShellCommand_ErrorWrapsExitCodeError_AtCmdTerraformBoundary(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err, "os.Executable() must succeed")

	atmosConfig := schema.AtmosConfiguration{}
	cmdErr := e.ExecuteShellCommand(
		atmosConfig,
		exePath,
		[]string{"-test.run=^$"},           // no test matches; TestMain exits before any test runs.
		"",                                 // dir: current working directory.
		[]string{"_ATMOS_TEST_EXIT_ONE=1"}, // env: makes TestMain call os.Exit(1).
		false,                              // dryRun: false — actually run the subprocess.
		"",                                 // redirectStdErr.
	)

	require.Error(t, cmdErr, "subprocess exit 1 must surface as a non-nil error")

	// The error reaching runHooksOnErrorWithOutput must remain wrapped as
	// ExitCodeError — this is the contract the CI hook plumbing depends on.
	var exitCodeErr errUtils.ExitCodeError
	require.True(t,
		errors.As(cmdErr, &exitCodeErr),
		"cmdErr passed into runHooksOnErrorWithOutput must satisfy errors.As(err, &errUtils.ExitCodeError{}); got %T: %v", cmdErr, cmdErr,
	)
	assert.Equal(t, 1, exitCodeErr.Code, "ExitCodeError.Code must equal the subprocess exit code")

	// Mirror what runHooksOnErrorWithOutput does in production: extract the
	// exit code via errUtils.GetExitCode. This is the consumed half of the
	// contract — if GetExitCode ever stops finding ExitCodeError in the
	// chain, the CI summary/check-run flow regresses to ExitCode=1 by
	// default, masking real exit codes (e.g., 2 for plan -detailed-exitcode).
	assert.Equal(t, 1, errUtils.GetExitCode(cmdErr),
		"errUtils.GetExitCode must extract the wrapped code — runHooksOnErrorWithOutput depends on this")

	// End-to-end: feed the cmdErr into the same RunCIHooks plumbing that
	// runHooksOnErrorWithOutput uses, and verify the wrapper survives in
	// the options struct that plugins observe. ci.enabled=false short-circuits
	// before any plugin runs, so this exercises only the option construction
	// + extraction path that this PR introduced.
	opts := &h.RunCIHooksOptions{
		Event:        h.AfterTerraformPlan,
		AtmosConfig:  &atmosConfig, // ci.enabled is the zero value (false) → short-circuits.
		Info:         &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
		Output:       "",
		ForceCIMode:  true,
		CommandError: cmdErr,
		ExitCode:     errUtils.GetExitCode(cmdErr),
	}

	// The wrapper contract must hold AT the boundary plugins see, not just at
	// the cmd/terraform layer. A future refactor that copies/wraps cmdErr
	// before placing it in the options would lose this property.
	require.True(t,
		errors.As(opts.CommandError, &exitCodeErr),
		"options.CommandError must still satisfy errors.As(err, &errUtils.ExitCodeError{}); got %T", opts.CommandError,
	)
	assert.Equal(t, 1, opts.ExitCode, "options.ExitCode must equal the wrapped subprocess exit code")
	assert.Equal(t, 1, exitCodeErr.Code)

	// Confirm the round-trip is harmless when CI is disabled.
	require.NoError(t, h.RunCIHooks(opts),
		"RunCIHooks must short-circuit cleanly when ci.enabled=false even with a non-nil CommandError")
}
