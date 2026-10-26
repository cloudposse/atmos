package shell

import (
	"os"
	"testing"
)

// TestMain implements the cross-platform "exit 0 / exit 1" subprocess pattern
// described in CLAUDE.md so RunCommand tests can spawn the test binary itself
// instead of relying on Unix-only `true` / `false` or PATH-dependent binaries.
// Tests set _ATMOS_SHELL_TEST_EXIT_OK=1 or _ATMOS_SHELL_TEST_EXIT_ONE=1 and use
// os.Executable() as the command.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_SHELL_TEST_EXIT_OK") == "1" {
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_SHELL_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	os.Exit(m.Run())
}
