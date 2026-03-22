package exec

import (
	"os"
	"testing"
)

// TestMain is the entry point for the internal/exec test binary.
// It intercepts the _ATMOS_TEST_EXIT_ONE env var before any test runs,
// enabling TestRunWorkspaceSetup_RecoveryPath to use the test binary itself
// as a cross-platform "command that always exits 1" — no Unix-only binaries required.
func TestMain(m *testing.M) {
	// Subprocess helper: when the test binary is invoked by TestRunWorkspaceSetup_RecoveryPath
	// as the "terraform" command, this env var causes it to exit 1 immediately, simulating
	// a failed "workspace select/new" without requiring the POSIX "false" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}
