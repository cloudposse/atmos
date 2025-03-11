package exec

import (
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPlanDiffCommandRouting tests that the plan-diff command is correctly routed
// in the ExecuteTerraform function's switch statement.
//
// NOTE: This test will show a linter error when run separately with golangci-lint,
// because ExecuteTerraform is defined in terraform.go and not visible when linting
// this file alone. However, it works correctly when running 'go test' because
// all files in the package are compiled together.
func TestPlanDiffCommandRouting(t *testing.T) {
	// Create minimal info with the plan-diff command
	info := schema.ConfigAndStacksInfo{
		Command:                "terraform",
		SubCommand:             "plan-diff",
		ComponentFromArg:       "test-component",
		Component:              "test-component",
		FinalComponent:         "test-component",
		ComponentIsEnabled:     true,
		AdditionalArgsAndFlags: []string{"--orig", "test.tfplan"},
	}

	// Execute the function - we expect it to fail because we haven't set up a proper environment
	err := ExecuteTerraform(info) // This line will show a linter error but works in tests

	// Verify we get an error since we have an incomplete setup
	if err == nil {
		t.Fatalf("Expected ExecuteTerraform to fail with incomplete setup")
	}

	// Check the error message to make sure it's not indicating that the command wasn't recognized
	errMsg := err.Error()
	t.Logf("Error: %v", errMsg)

	// If the error indicates that the command wasn't recognized, the test should fail
	if strings.Contains(errMsg, "unknown command") ||
		strings.Contains(errMsg, "unrecognized command") ||
		strings.Contains(errMsg, "invalid command") {
		t.Errorf("Error suggests the plan-diff command wasn't recognized in the switch statement")
	}
}
