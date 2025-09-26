package tests

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func TestExecuteDescribeComponentCmd_Success_YAMLWithPager(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-include-yaml-function")
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod", "--pager=more", "--format", "yaml"}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute command: %v", err)
	}
}
