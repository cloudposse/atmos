package tests

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	// This test uses a fixture that downloads from GitHub, so check rate limits first.
	RequireGitHubAccess(t)

	// Skip in CI environments without TTY.
	// The pager functionality requires /dev/tty to be accessible.
	if _, err := os.Open("/dev/tty"); err != nil {
		t.Skipf("Skipping test: TTY not available (/dev/tty): %v", err)
	}

	t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}
}
