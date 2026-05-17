package atmos

import (
	"os"
	"testing"
)

// TestMain is the entry point for the pkg/ai/tools/atmos test binary.
// It intercepts env vars before any test runs, enabling tests to use
// the test binary itself as a portable subprocess — no Unix-only binaries required.
//
// Supported env vars:
//
//	_ATMOS_TEST_EXIT_ZERO=1 — exit 0 immediately (simulates a successful command).
//	_ATMOS_TEST_EXIT_ONE=1  — exit 1 immediately (simulates a failed command).
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_EXIT_ZERO") == "1" {
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}
