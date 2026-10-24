package ansible

import (
	"fmt"
	"os"
	"testing"
)

// TestMain is the entry point for the pkg/component/ansible test binary.
// It intercepts a few env vars before any test runs, enabling tests to use the test
// binary itself as a portable "ansible-playbook" subprocess -- no real ansible install
// or Unix-only binaries required. Mirrors the equivalent gate in
// internal/exec/testmain_test.go.
//
// Supported env vars (processed in declaration order).
//
//	_ATMOS_TEST_COUNTER_FILE=<path>  — if set, append one byte ("x") to <path> on every
//	                                   invocation (lets tests count subprocess invocations).
//	_ATMOS_TEST_EXIT_ONE=1           — if set, exit 1 (writing _ATMOS_TEST_STDOUT/
//	                                   _ATMOS_TEST_STDERR first when also set) -- used to
//	                                   drive retry.conditions matching in end-to-end
//	                                   retry-wiring tests.
func TestMain(m *testing.M) {
	if counterFile := os.Getenv("_ATMOS_TEST_COUNTER_FILE"); counterFile != "" {
		fd, err := os.OpenFile(counterFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open counter file: %v\n", err)
			os.Exit(1)
		}
		if _, err := fd.WriteString("x"); err != nil {
			fmt.Fprintf(os.Stderr, "write counter file: %v\n", err)
			os.Exit(1)
		}
		if err := fd.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "close counter file: %v\n", err)
			os.Exit(1)
		}
	}

	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		if stdout := os.Getenv("_ATMOS_TEST_STDOUT"); stdout != "" {
			_, _ = os.Stdout.WriteString(stdout)
		}
		if stderr := os.Getenv("_ATMOS_TEST_STDERR"); stderr != "" {
			_, _ = os.Stderr.WriteString(stderr)
		}
		os.Exit(1)
	}

	os.Exit(m.Run())
}
