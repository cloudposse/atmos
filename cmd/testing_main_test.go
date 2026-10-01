package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain provides package-level test setup and teardown.
// It ensures RootCmd state is properly managed across all tests in the package.
func TestMain(m *testing.M) {
	// Cross-platform subprocess helper: exit with code 1 when env flag is set.
	// This lets tests use the test binary itself as a cross-platform "exit 1" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	// Cross-platform subprocess helper: when _ATMOS_TEST_DUMP_ENV names a file, write this
	// process's environment (one KEY=VALUE per line) to it and exit. This lets tests capture the
	// environment Atmos passes to a custom-command step subprocess without invoking
	// platform-specific binaries (e.g. `env` / `cmd /c set`).
	if dumpFile := os.Getenv("_ATMOS_TEST_DUMP_ENV"); dumpFile != "" {
		if err := os.WriteFile(dumpFile, []byte(strings.Join(os.Environ(), "\n")), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	wroteOutput := false
	if stdout := os.Getenv("_ATMOS_TEST_STDOUT"); stdout != "" {
		_, _ = os.Stdout.WriteString(stdout)
		wroteOutput = true
	}
	if stderr := os.Getenv("_ATMOS_TEST_STDERR"); stderr != "" {
		_, _ = os.Stderr.WriteString(stderr)
		wroteOutput = true
	}
	if wroteOutput {
		os.Exit(0)
	}

	// Initialize the I/O writer and ui formatter so data.Write*/ui.Write* calls
	// (used throughout cmd/root.go and its helpers) don't panic or silently
	// no-op during tests.
	ioCtx, err := iolib.NewContext()
	if err != nil {
		panic("cmd tests: failed to create IO context: " + err.Error())
	}
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

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
