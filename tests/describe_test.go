package tests

import (
	"os"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

// skipIfNoTTY skips the test if TTY is not available.
// Uses cross-platform TTY detection and properly closes file handles.
func skipIfNoTTY(t *testing.T) {
	t.Helper()

	// Skip if stdout doesn't support TTY.
	if !term.IsTTYSupportForStdout() {
		t.Skip("Skipping test: TTY not supported for stdout")
	}

	// On Unix-like systems, also check /dev/tty accessibility for alternate screen mode.
	if runtime.GOOS != "windows" {
		if f, err := os.Open("/dev/tty"); err != nil {
			t.Skipf("Skipping test: /dev/tty not accessible: %v", err)
		} else {
			f.Close()
		}
	}
}

func TestDescribeComponentJSON(t *testing.T) {
	skipIfNoTTY(t)

	// Set up the environment variables.
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--format", "json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}

func TestDescribeComponentYAML(t *testing.T) {
	skipIfNoTTY(t)

	// Set up the environment variables.
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--format", "yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}
