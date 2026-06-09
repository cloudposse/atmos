package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDefaultStacksProcessor_ExecuteDescribeStacks verifies that DefaultStacksProcessor correctly delegates to ExecuteDescribeStacks.
func TestDefaultStacksProcessor_ExecuteDescribeStacks(t *testing.T) {
	// Create a test configuration directory.
	testDir := t.TempDir()

	// Create minimal atmos.yaml.
	atmosConfig := schema.AtmosConfiguration{
		BasePath: testDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	processor := &DefaultStacksProcessor{}

	// Call ExecuteDescribeStacks with empty parameters.
	// This should not error even with no stack files, as we're ignoring missing files.
	result, err := processor.ExecuteDescribeStacks(
		&atmosConfig,
		"",         // filterByStack
		[]string{}, // components
		[]string{}, // componentTypes
		[]string{}, // sections
		true,       // ignoreMissingFiles
		false,      // processTemplates
		false,      // processYamlFunctions
		false,      // includeEmptyStacks
		[]string{}, // skip
		nil,        // authManager
	)

	// With no stacks and ignoreMissingFiles=true, we expect success with empty result.
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// TestDefaultStacksProcessor_ExecuteDescribeStacksWithAuthDisabled verifies the
// auth-disabled variant delegates to ExecuteDescribeStacksWithAuthDisabled. The
// method is pure pass-through (added so callers can route `--identity=false`
// through the StacksProcessor seam in pkg/list), so a single call confirms
// every line of the delegation is reached.
func TestDefaultStacksProcessor_ExecuteDescribeStacksWithAuthDisabled(t *testing.T) {
	testDir := t.TempDir()

	atmosConfig := schema.AtmosConfiguration{
		BasePath: testDir,
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	processor := &DefaultStacksProcessor{}

	// Use the same empty-fixture shape as TestDefaultStacksProcessor_ExecuteDescribeStacks
	// so this test only proves the delegation; correctness of the underlying
	// ExecuteDescribeStacksWithAuthDisabled is covered in the auth-disabled tests
	// in internal/exec/describe_stacks_component_processor_auth_test.go.
	tests := []struct {
		name         string
		authDisabled bool
	}{
		{name: "authDisabled=true short-circuits per-component auth", authDisabled: true},
		{name: "authDisabled=false matches the non-auth-disabled call shape", authDisabled: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.ExecuteDescribeStacksWithAuthDisabled(
				&atmosConfig,
				"",         // filterByStack
				[]string{}, // components
				[]string{}, // componentTypes
				[]string{}, // sections
				true,       // ignoreMissingFiles
				false,      // processTemplates
				false,      // processYamlFunctions
				false,      // includeEmptyStacks
				[]string{}, // skip
				nil,        // authManager
				tt.authDisabled,
			)
			assert.NoError(t, err)
			assert.NotNil(t, result)
		})
	}
}
