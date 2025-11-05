package tests

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	// Use TestKit to isolate RootCmd state and restore os.Args.
	// This prevents test framework flags (like -test.timeout) from leaking into the command.
	_ = testhelpers.NewTestKit(t)

	t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")

	// Set os.Args to match the command we're testing.
	// This is required because UsageFunc reads os.Args directly when DisableFlagParsing=true.
	// TestKit will restore the original os.Args after the test.
	os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"}

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}
}
