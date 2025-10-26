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
	)

	// With no stacks and ignoreMissingFiles=true, we expect success with empty result.
	assert.NoError(t, err)
	assert.NotNil(t, result)
}
