package tests

import (
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/cloudposse/atmos/tests/testhelpers"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	// Use TestKit to isolate RootCmd state.
	_ = testhelpers.NewTestKit(t, cmd.RootCmd)

	t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}
}
