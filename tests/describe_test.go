package tests

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func TestDescribeComponentJSON(t *testing.T) {
	// Skip in CI environments without TTY.
	// The pager functionality requires /dev/tty to be accessible.
	if _, err := os.Open("/dev/tty"); err != nil {
		t.Skipf("Skipping test: TTY not available (/dev/tty): %v", err)
	}

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
	// Skip in CI environments without TTY.
	// The pager functionality requires /dev/tty to be accessible.
	if _, err := os.Open("/dev/tty"); err != nil {
		t.Skipf("Skipping test: TTY not available (/dev/tty): %v", err)
	}

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
