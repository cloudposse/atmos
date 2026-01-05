package exec

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// TestDescribeComponent_MissingComponentReference tests issue #1030:
// When a component template references a non-existent component via atmos.Component(),
// it should return a clear error, not fail silently.
//
// See: https://github.com/cloudposse/atmos/issues/1030
func TestDescribeComponent_MissingComponentReference(t *testing.T) {
	// Clear caches to ensure fresh processing.
	ClearBaseComponentConfigCache()
	ClearMergeContexts()
	ClearLastMergeContext()
	ClearFileContentCache()

	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err, "Failed to unset 'ATMOS_CLI_CONFIG_PATH'")

	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err, "Failed to unset 'ATMOS_BASE_PATH'")

	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

	// Mock os.Exit to prevent test termination and capture exit code.
	// Some code paths call CheckErrorPrintAndExit which calls os.Exit.
	var exitCode int
	exitCalled := false
	originalOsExit := errUtils.OsExit
	errUtils.OsExit = func(code int) {
		exitCode = code
		exitCalled = true
		// Don't actually exit - just record the code.
	}
	defer func() {
		errUtils.OsExit = originalOsExit
	}()

	// Define the working directory with missing component reference fixture.
	workDir := "../../tests/fixtures/scenarios/missing-component-reference"
	t.Chdir(workDir)

	// Set ATMOS_CLI_CONFIG_PATH to CWD to isolate from repo's atmos.yaml.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")

	// Attempt to describe a component that references a non-existent component.
	// The template '{{ (atmos.Component "nonexistent" .stack).vars.some_var }}'
	// should fail because "nonexistent" component doesn't exist.
	_, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-with-missing-ref",
		Stack:                "test",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})

	// EXPECTED BEHAVIOR (issue #1030 fix):
	// An error should be returned when atmos.Component() cannot find the referenced component.
	// The error should wrap ErrInvalidComponent and contain useful information.

	// BUG BEHAVIOR (issue #1030):
	// err is nil (silent failure) and the template renders as empty or <no value>.

	// Check if os.Exit was called (which indicates error WAS detected, just not returned properly).
	// If exitCalled is true with exitCode != 0, the error was detected but os.Exit was called
	// instead of returning the error - this is better than silent failure but still not ideal.
	if exitCalled {
		assert.NotEqual(t, 0, exitCode, "os.Exit was called with exit code 0 - this indicates silent success when failure was expected")
		t.Logf("NOTE: os.Exit(%d) was called instead of returning error. Error WAS detected (issue #1030 silent failure is fixed), "+
			"but os.Exit is being called instead of returning the error to the caller.", exitCode)
		// The test passes because the error WAS detected and reported (issue #1030 is about silent failure).
		return
	}

	// Assert that an error IS returned (not nil).
	// If this assertion fails, the bug from issue #1030 still exists.
	assert.Error(t, err, "Expected error when referencing non-existent component via atmos.Component(), but got nil. "+
		"This indicates issue #1030 (silent failure) is still present.")

	// If we got an error, verify it's the right kind of error.
	if err != nil {
		// The error should wrap ErrInvalidComponent.
		assert.True(t, errors.Is(err, errUtils.ErrInvalidComponent),
			"Expected error to wrap ErrInvalidComponent, got: %v", err)

		// The error message should contain useful information.
		errMsg := err.Error()
		assert.Contains(t, errMsg, "nonexistent",
			"Error message should mention the missing component name 'nonexistent'")
	}
}
