package cmd

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestMain provides package-level test setup and teardown.
// It ensures RootCmd state is properly managed across all tests in the package.
func TestMain(m *testing.M) {
	// Capture initial RootCmd state using shared testhelpers.
	initialSnapshot := testhelpers.SnapshotCobraState(RootCmd)

	// Run all tests.
	exitCode := m.Run()

	// Restore RootCmd to initial state after all tests complete.
	// This ensures the package leaves no pollution for other test packages.
	testhelpers.RestoreCobraState(RootCmd, initialSnapshot)

	// Exit with the test result code.
	os.Exit(exitCode)
}
