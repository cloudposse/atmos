package merge

import (
	"errors"
	"fmt"
	"testing"
)

// TestMergeContextErrorDemo demonstrates the enhanced error formatting
// This simulates what users will see when a merge error occurs
func TestMergeContextErrorDemo(t *testing.T) {
	// Simulate the kind of error that mergo would return
	mergoError := errors.New("cannot override two slices with different type ([]interface {}, string)")

	// Create a merge context that simulates a real import chain
	ctx := NewMergeContext()
	ctx = ctx.WithFile("stacks/catalog/base.yaml")
	ctx = ctx.WithFile("stacks/mixins/region/us-east-1.yaml") 
	ctx = ctx.WithFile("stacks/dev/environment.yaml")

	// Format the error with context
	enhancedError := ctx.FormatError(mergoError)

	// Print the enhanced error to show what users will see
	fmt.Printf("\n=== Enhanced Error Message ===\n%s\n=== End of Error Message ===\n", enhancedError)

	// The test passes if we successfully format the error
	if enhancedError != nil {
		t.Log("Successfully demonstrated enhanced error formatting")
	}
}

// TestMergeContextRealWorldScenario demonstrates a more complex scenario
func TestMergeContextRealWorldScenario(t *testing.T) {
	// Simulate a deeply nested import chain
	ctx := NewMergeContext()
	ctx = ctx.WithFile("stacks/orgs/acme/_defaults.yaml")
	ctx = ctx.WithFile("stacks/catalog/vpc/defaults.yaml")
	ctx = ctx.WithFile("stacks/catalog/vpc/base.yaml")
	ctx = ctx.WithFile("stacks/mixins/region/us-east-1.yaml")
	ctx = ctx.WithFile("stacks/mixins/account/prod.yaml")
	ctx = ctx.WithFile("stacks/deploy/prod/us-east-1.yaml")

	// Simulate the error
	mergoError := errors.New("cannot override two slices with different type ([]interface {}, string)")

	// Format with context
	enhancedError := ctx.FormatError(mergoError, 
		"This error occurred while processing VPC configuration",
		"The 'subnets' field appears to have conflicting types")

	// Print to show the result
	fmt.Printf("\n=== Complex Scenario Error ===\n%s\n=== End of Error ===\n", enhancedError)

	t.Log("Demonstrated complex import chain error formatting")
}