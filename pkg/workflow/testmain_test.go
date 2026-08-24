package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// controlBridgeFakeChildEnv gates a fake child process used by
// control_bridge_run_command_test.go to exercise plainControlRunCommand
// cross-platform, without depending on a real "atmos" binary or a
// platform-specific shell. The test binary impersonates the child: it writes
// a marker file into its current directory (proving Dir was honored — Windows
// can report os.Getwd() using an 8.3 short name that doesn't string-match the
// long path a caller resolved, so callers must check for this file rather
// than compare cwd strings) and prints a caller-supplied marker to stdout
// (proving Env was honored), and — when controlBridgeFakeChildFailEnv is also
// set — writes to stderr and exits non-zero.
const (
	controlBridgeFakeChildEnv           = "_ATMOS_WORKFLOW_CONTROL_FAKE"
	controlBridgeFakeChildFailEnv       = "_ATMOS_WORKFLOW_CONTROL_FAKE_FAIL"
	controlBridgeFakeChildMarker        = "_ATMOS_WORKFLOW_CONTROL_MARKER"
	controlBridgeFakeChildCwdMarkerFile = "fake-child-cwd-marker"
)

func TestMain(m *testing.M) {
	if os.Getenv(controlBridgeFakeChildEnv) == "1" {
		runControlBridgeFakeChild()
	}
	os.Exit(m.Run())
}

func runControlBridgeFakeChild() {
	if cwd, err := os.Getwd(); err == nil {
		_ = os.WriteFile(filepath.Join(cwd, controlBridgeFakeChildCwdMarkerFile), nil, 0o600)
	}
	fmt.Fprintf(os.Stdout, "fake-child-stdout marker=%s\n", os.Getenv(controlBridgeFakeChildMarker))
	if os.Getenv(controlBridgeFakeChildFailEnv) == "1" {
		fmt.Fprintln(os.Stderr, "fake-child-stderr")
		os.Exit(3)
	}
	os.Exit(0)
}
