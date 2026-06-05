package hooks

import (
	"fmt"
	"os"
	"testing"
)

// TestMain is the package's test entry point. It checks for two env-gate
// hooks before running the suite:
//
//   - _ATMOS_TEST_EXIT_ONE: exit with code 1 immediately. Lets tests use the
//     test binary itself as a cross-platform "exit 1" command (no /bin/false).
//   - _ATMOS_TEST_WRITE_OUTPUT: write the value of _ATMOS_TEST_OUTPUT_BODY to
//     the path in $ATMOS_OUTPUT_FILE, then exit 0. Lets tests simulate a tool
//     that produces structured side-channel output.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" {
		os.Exit(1)
	}
	if os.Getenv("_ATMOS_TEST_WRITE_OUTPUT") == "1" {
		out := os.Getenv("ATMOS_OUTPUT_FILE")
		body := os.Getenv("_ATMOS_TEST_OUTPUT_BODY")
		if out != "" {
			if err := os.WriteFile(out, []byte(body), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
