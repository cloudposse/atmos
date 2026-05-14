package terraform

import (
	"os"
	"testing"
)

// TestMain is the entry point for the cmd/terraform test binary.
// It intercepts subprocess-helper env vars before any test runs, enabling
// tests to use the test binary itself as a portable cross-platform subprocess
// (no Unix-only binaries required).
//
// Supported env vars:
//
//	_ATMOS_TEST_EXIT_ONE=1 — if set, exit 1 immediately so the parent process
//	                         observes a non-zero exit code without invoking
//	                         any actual test. Used by the ExitCodeError
//	                         wrapping-contract test in
//	                         utils_exit_wrapping_test.go.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}
