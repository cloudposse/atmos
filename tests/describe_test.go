package tests

import (
	"os"
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

func ExecuteCommand(args []string) error {
	// Set the command line arguments
	os.Args = args

	// Execute the command
	return cmd.Execute()
}

func TestDescribeComponentJSON(t *testing.T) {
	// Set up the environment variables
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Execute the command
	os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod", "--format", "json"}
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}

func TestDescribeComponentYAML(t *testing.T) {
	// Set up the environment variables
	t.Chdir("./fixtures/scenarios/atmos-providers-section")

	t.Setenv("ATMOS_PAGER", "more")

	// Execute the command
	os.Args = []string{"atmos", "describe", "component", "component-1", "--stack", "nonprod", "--format", "yaml"}
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}
