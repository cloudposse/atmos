package hooks

import (
	"fmt"
	"os"
	"testing"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// TestMain is the package's test entry point. It checks for two env-gate
// hooks before running the suite:
//
//   - _ATMOS_TEST_EXIT_ONE: exit with code 1 immediately. Lets tests use the
//     test binary itself as a cross-platform "exit 1" command (no /bin/false).
//   - _ATMOS_TEST_WRITE_OUTPUT: write the value of _ATMOS_TEST_OUTPUT_BODY to
//     the path in $ATMOS_OUTPUT_FILE, then exit 0. Lets tests simulate a tool
//     that produces structured side-channel output.
//   - _ATMOS_TEST_ECHO_STDOUT: write the value of _ATMOS_TEST_STDOUT_BODY to
//     os.Stdout, then exit 0. Lets tests simulate a tool that emits structured
//     output to stdout (e.g. tflint --format=sarif) so the engine's
//     CaptureStdout redirect can be verified cross-platform via os.Executable().
//   - _ATMOS_TEST_ECHO_STDERR: write the value of _ATMOS_TEST_STDERR_BODY to
//     os.Stderr, then exit 0. Lets tests verify subprocess stderr routing.
//   - _ATMOS_TEST_WRITE_CWD: write the subprocess working directory and
//     ATMOS_COMPONENT_PATH to ATMOS_OUTPUT_FILE, separated by a newline.
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
	}
	if os.Getenv("_ATMOS_TEST_ECHO_STDERR") == "1" {
		fmt.Fprint(os.Stderr, os.Getenv("_ATMOS_TEST_STDERR_BODY"))
	}
	if os.Getenv("_ATMOS_TEST_ECHO_STDOUT") == "1" || os.Getenv("_ATMOS_TEST_ECHO_STDERR") == "1" {
		os.Exit(0)
	}
	if os.Getenv("_ATMOS_TEST_WRITE_CWD") == "1" {
		out := os.Getenv("ATMOS_OUTPUT_FILE")
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(out, []byte(wd+"\n"+os.Getenv("ATMOS_COMPONENT_PATH")), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Initialize the I/O writer and ui formatter so data.Write*/ui.Write* calls
	// (used by pkg/ci CI annotations/log groups, exercised via command_engine.go)
	// don't panic or silently no-op during tests.
	if ioCtx, err := iolib.NewContext(); err == nil {
		data.InitWriter(ioCtx)
		ui.InitFormatter(ioCtx)
	}

	os.Exit(m.Run())
}
