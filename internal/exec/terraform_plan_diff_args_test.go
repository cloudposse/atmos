package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGenerateNewPlanFileArgs verifies that -var flags are preserved when generating a new plan file.
// This test doesn't actually run terraform, it just verifies the argument construction.
func TestGenerateNewPlanFileArgs(t *testing.T) {
	// Create a test info object simulating the plan-diff command with -var flag
	info := &schema.ConfigAndStacksInfo{
		SubCommand: "plan-diff",
		AdditionalArgsAndFlags: []string{
			"--orig=orig.plan",
			"-var",
			"foo=new-value",
		},
	}

	// Filter the flags as done in generateNewPlanFile
	planArgs := filterPlanDiffFlags(info.AdditionalArgsAndFlags)

	// Verify that -var flag is preserved
	assert.Contains(t, planArgs, "-var", "Expected -var flag to be preserved")
	assert.Contains(t, planArgs, "foo=new-value", "Expected var value to be preserved")

	// Verify that --orig flag is filtered out
	assert.NotContains(t, planArgs, "--orig=orig.plan", "Expected --orig flag to be filtered out")

	t.Logf("Filtered args for terraform plan: %v", planArgs)
}
