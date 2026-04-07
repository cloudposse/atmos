package cmd

import (
	"os"
	"testing"
)

// TestMain provides package-level test setup and teardown.
// It ensures RootCmd state is properly managed across all tests in the package.
func TestMain(m *testing.M) {
	// Cross-platform subprocess helper: exit with code 1 when env flag is set.
	// This lets tests use the test binary itself as a cross-platform "exit 1" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	// Capture initial RootCmd state.
	initialSnapshot := snapshotRootCmdState()

	// Run all tests.
	exitCode := m.Run()

	// Restore RootCmd to initial state after all tests complete.
	// This ensures the package leaves no pollution for other test packages.
	restoreRootCmdState(initialSnapshot)

	// Exit with the test result code.
	os.Exit(exitCode)
}
