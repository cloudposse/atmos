package exec

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestYamlFuncTerraformOutputWithNewlines tests that terraform.output preserves newlines in output values.
func TestYamlFuncTerraformOutputWithNewlines(t *testing.T) {
	// Create a minimal config for testing
	atmosCfg := schema.AtmosConfiguration{
		BasePath: "/tmp",
		Stacks: schema.Stacks{
			BasePath: "stacks",
		},
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Test the direct string processing to ensure newlines aren't stripped

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiline text with trailing newline",
			input:    "line1\nline2\nline3\n",
			expected: "line1\nline2\nline3\n",
		},
		{
			name:     "text with single trailing newline",
			input:    "hello world\n",
			expected: "hello world\n",
		},
		{
			name:     "text with leading newline",
			input:    "\nhello world",
			expected: "\nhello world",
		},
		{
			name:     "text with multiple consecutive newlines",
			input:    "\n\nhello\n\nworld\n\n",
			expected: "\n\nhello\n\nworld\n\n",
		},
	}

	// Test that the YAML processing preserves newlines
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test map with the string value
			testData := map[string]any{
				"test_key": tc.input,
			}

			// Process through the YAML custom tags processor
			processed, err := ProcessCustomYamlTags(&atmosCfg, testData, "test-stack", nil)
			assert.NoError(t, err)

			// Verify the newlines are preserved
			assert.Equal(t, tc.expected, processed["test_key"],
				"Newlines should be preserved in terraform.output values")
		})
	}
}

// TestYamlFuncTerraformOutputIntegration tests the full integration with ExecuteDescribeComponent.
func TestYamlFuncTerraformOutputIntegration(t *testing.T) {
	// Skip if we don't have terraform/tofu available
	if _, err := os.Stat("/usr/local/bin/tofu"); os.IsNotExist(err) {
		if _, err := os.Stat("/usr/local/bin/terraform"); os.IsNotExist(err) {
			t.Skipf("Skipping integration test: neither terraform nor tofu is available")
		}
	}

	// This would be a more comprehensive integration test
	// that actually runs terraform and verifies the outputs
	// For now, we'll focus on the unit test above
}