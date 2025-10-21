package tests

import (
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func TestDescribeComponentJSON(t *testing.T) {
	// Set up the environment variables
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
	// Set up the environment variables
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Use SetArgs for Cobra command testing.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "component-1", "--stack", "nonprod", "--format", "yaml"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}
