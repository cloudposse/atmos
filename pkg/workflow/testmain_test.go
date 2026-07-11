package workflow

import (
	"fmt"
	"os"
	"testing"
)

// controlBridgeFakeChildEnv gates a fake child process used by
// control_bridge_run_command_test.go to exercise plainControlRunCommand
// cross-platform, without depending on a real "atmos" binary or a
// platform-specific shell. The test binary impersonates the child: it prints
// its working directory and a caller-supplied marker to stdout (proving Dir
// and Env were honored), and — when controlBridgeFakeChildFailEnv is also
// set — writes to stderr and exits non-zero.
const (
	controlBridgeFakeChildEnv     = "_ATMOS_WORKFLOW_CONTROL_FAKE"
	controlBridgeFakeChildFailEnv = "_ATMOS_WORKFLOW_CONTROL_FAKE_FAIL"
	controlBridgeFakeChildMarker  = "_ATMOS_WORKFLOW_CONTROL_MARKER"
)

func TestMain(m *testing.M) {
	if os.Getenv(controlBridgeFakeChildEnv) == "1" {
		runControlBridgeFakeChild()
	}
	os.Exit(m.Run())
}

func runControlBridgeFakeChild() {
	cwd, _ := os.Getwd()
	fmt.Fprintf(os.Stdout, "fake-child-stdout cwd=%s marker=%s\n", cwd, os.Getenv(controlBridgeFakeChildMarker))
	if os.Getenv(controlBridgeFakeChildFailEnv) == "1" {
		fmt.Fprintln(os.Stderr, "fake-child-stderr")
		os.Exit(3)
	}
	os.Exit(0)
}
