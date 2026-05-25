package exec

import (
	"os"
	"testing"
)

// TestMain is the entry point for the internal/exec test binary.
// It intercepts several env vars before any test runs, enabling tests to use
// the test binary itself as a portable subprocess — no Unix-only binaries required.
//
// Supported env vars (processed in declaration order):
//
//	_ATMOS_TEST_COUNTER_FILE=<path>  — if set, append one byte ("x") to <path>
//	                                   on every invocation (for single-invocation
//	                                   regression guard in terraform_execute_single_invocation_test.go).
//	_ATMOS_TEST_ENV_DUMP_FILE=<path> — if set, write the subprocess env (one
//	                                   KEY=VALUE per line) to <path> and exit 0.
//	                                   Used to inspect the actual env passed to a
//	                                   subprocess (e.g., to verify TF_CLI_ARGS
//	                                   sanitization at the workspace-setup step).
//	_ATMOS_TEST_EXIT_ONE=1           — if set, exit 1 immediately after the optional
//	                                   counter-file write (for workspace recovery tests).
func TestMain(m *testing.M) {
	// Write a single byte to the counter file on every invocation.
	// This lets tests count how many times the subprocess was spawned by reading
	// the file length: len(file) == number of invocations.
	if counterFile := os.Getenv("_ATMOS_TEST_COUNTER_FILE"); counterFile != "" {
		fd, err := os.OpenFile(counterFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fd.WriteString("x")
			_ = fd.Close()
		}
	}

	// Subprocess helper: dump the entire process environment (one KEY=VALUE entry per line)
	// to the given file and exit 0.  Used by tests that need to verify the exact env vars
	// a subprocess receives — e.g., the TF_CLI_ARGS workspace-sanitization test.
	if dumpFile := os.Getenv("_ATMOS_TEST_ENV_DUMP_FILE"); dumpFile != "" {
		fd, err := os.OpenFile(dumpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err == nil {
			for _, kv := range os.Environ() {
				_, _ = fd.WriteString(kv + "\n")
			}
			_ = fd.Close()
		}
		os.Exit(0)
	}

	// Subprocess helper: when the test binary is invoked as the "terraform" command,
	// this env var causes it to exit 1 immediately, simulating a failed workspace
	// command without requiring the POSIX "false" command.
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}
