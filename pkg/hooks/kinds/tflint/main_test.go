package tflint

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestMain lets the test binary itself stand in for the `tflint` executable
// (cross-platform, no reliance on a real tflint install):
//
//   - _ATMOS_TEST_TFLINT_FAKE: write os.Args[1:] (space-joined) to the file
//     named by _ATMOS_TEST_ARGS_FILE (when set), then emit
//     _ATMOS_TEST_STDOUT_BODY to stdout and exit 0. Lets a test verify both
//     which args tflintEngine actually resolved and that CommandEngine's
//     CaptureStdout redirect captured them.
func TestMain(m *testing.M) {
	if os.Getenv("_ATMOS_TEST_TFLINT_FAKE") == "1" {
		if argsFile := os.Getenv("_ATMOS_TEST_ARGS_FILE"); argsFile != "" {
			_ = os.WriteFile(argsFile, []byte(strings.Join(os.Args[1:], "\n")), 0o600)
		}
		fmt.Fprint(os.Stdout, os.Getenv("_ATMOS_TEST_STDOUT_BODY"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}
