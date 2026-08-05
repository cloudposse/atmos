package scanners

import (
	"fmt"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain lets the test binary impersonate a fake scanner command. Mirrors
// the same env-gate convention used by pkg/hooks (see pkg/hooks/main_test.go)
// so scanners.Run can be exercised cross-platform via os.Executable() instead
// of depending on a real scanner binary such as tflint or trivy being installed.
//
//   - _ATMOS_TEST_EXIT_ONE: exit with code 1 immediately.
//   - _ATMOS_TEST_WRITE_OUTPUT: write _ATMOS_TEST_OUTPUT_BODY to the path in
//     $ATMOS_OUTPUT_FILE, then exit 0.
//   - _ATMOS_TEST_ECHO_STDOUT: write _ATMOS_TEST_STDOUT_BODY to stdout, then
//     exit 0 — simulates a tool like tflint that emits structured output to
//     stdout instead of a file.
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
	if os.Getenv("_ATMOS_TEST_ECHO_STDOUT") == "1" {
		fmt.Fprint(os.Stdout, os.Getenv("_ATMOS_TEST_STDOUT_BODY"))
		os.Exit(0)
	}

	// Initialize the I/O writer and ui formatter so data.Write*/ui.Write* calls
	// (exercised via renderTerminal) don't panic or silently no-op in tests.
	if ioCtx, err := iolib.NewContext(); err == nil {
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	}

	os.Exit(m.Run())
}
