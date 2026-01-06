package tests

import (
	"testing"

	"github.com/cloudposse/atmos/cmd"
)

// TestLocalsResolutionDev tests that file-scoped locals are properly resolved in dev environment.
// This is an integration test for GitHub issue #1933.
func TestLocalsResolutionDev(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")
	t.Setenv("ATMOS_PAGER", "more")

	// Run describe component command.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "mock/instance-1", "--stack", "dev-us-east-1", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}

// TestLocalsResolutionProd tests locals resolution in the prod environment.
func TestLocalsResolutionProd(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")
	t.Setenv("ATMOS_PAGER", "more")

	// Run describe component command.
	cmd.RootCmd.SetArgs([]string{"describe", "component", "mock/primary", "--stack", "prod-us-east-1", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}

// TestLocalsDescribeStacks tests that describe stacks works with locals.
func TestLocalsDescribeStacks(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals")
	t.Setenv("ATMOS_PAGER", "more")

	// Run describe stacks command.
	cmd.RootCmd.SetArgs([]string{"describe", "stacks", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}

// TestLocalsCircularDependency verifies that circular locals don't crash the system.
// When locals have a cycle, the resolver should log an error and continue without locals.
func TestLocalsCircularDependency(t *testing.T) {
	t.Chdir("./fixtures/scenarios/locals-circular")
	t.Setenv("ATMOS_PAGER", "more")

	// Run describe stacks command - should succeed even with circular locals.
	// The circular locals are logged as a trace warning but processing continues.
	cmd.RootCmd.SetArgs([]string{"describe", "stacks", "--format", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
}
