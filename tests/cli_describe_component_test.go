package tests

import (
	"os"
	"runtime"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	// This test uses a fixture that downloads from GitHub, so check rate limits first.
	RequireGitHubAccess(t)

	// Skip in CI environments without TTY.
	// The pager functionality requires TTY support.
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

	t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}
}
