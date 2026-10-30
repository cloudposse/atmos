package cmd

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/auth/credentials/credtest"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain provides package-level test setup and teardown.
// It ensures RootCmd state is properly managed across all tests in the package.
func TestMain(m *testing.M) {
	// Capture nested Atmos arguments without requiring an installed Atmos binary.
	if path := os.Getenv("_ATMOS_TEST_DUMP_ARGS"); path != "" {
		args, err := json.Marshal(os.Args[1:])
		if err != nil || os.WriteFile(path, args, 0o600) != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Capture component executable arguments without requiring a platform-specific shell.
	if marker := os.Getenv("_ATMOS_TEST_COMPONENT_ARGS"); marker != "" {
		if stderr := os.Getenv("_ATMOS_TEST_COMPONENT_STDERR"); stderr != "" {
			_, _ = os.Stderr.WriteString(stderr)
		}
		if err := os.WriteFile(marker, []byte(strings.Join(os.Args[1:], "\n")), 0o600); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Cross-platform subprocess helper: exit with code 1 when env flag is set.
	// This lets tests use the test binary itself as a cross-platform "exit 1" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}

	if os.Getenv("_ATMOS_PLUGIN_COMMAND_HELPER") == "1" {
		os.Exit(runPluginCommandHelper())
	}

	// Cross-platform subprocess helper: sleep for the given milliseconds, then exit 0. This lets
	// tests exercise step timeouts with the test binary itself as a long-running command.
	if sleepMS := os.Getenv("_ATMOS_TEST_SLEEP_MS"); sleepMS != "" {
		if ms, err := strconv.Atoi(sleepMS); err == nil {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		os.Exit(0)
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
	// Never touch the real OS keychain: default the credential store to an in-memory keyring.
	exitCode := credtest.IsolateKeyringMain(m)

	// Restore RootCmd to initial state after all tests complete.
	// This ensures the package leaves no pollution for other test packages.
	restoreRootCmdState(initialSnapshot)

	// Exit with the test result code.
	os.Exit(exitCode)
}
