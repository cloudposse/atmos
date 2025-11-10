package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestYamlFunctionsInLists tests that YAML functions work correctly when used in lists.
func TestYamlFunctionsInLists(t *testing.T) {
	// Test case 1: Simple list with terraform.output functions
	yamlContent1 := `
test_list:
  - !terraform.output component1 stack1 output1
  - !terraform.output component2 stack2 output2
  - !terraform.output component3 stack3 output3
`

	// Test case 2: Mixed list with functions and static values
	yamlContent2 := `
mixed_list:
  - "static-value-1"
  - !terraform.output component1 stack1 output1
  - "static-value-2"
  - !terraform.output component2 stack2 output2
`

	// Test case 3: List with terraform.state functions (the reported issue)
	yamlContent3 := `
ecr_repository_arns:
  - !terraform.state image1-ecr global repository_arn
  - !terraform.state image2-ecr global repository_arn
`

	// Test case 4: Nested structure with lists containing functions
	yamlContent4 := `
import:
  - ./_defaults
  - path: catalog/ecr-deployer-role
    context:
      ecr_repository_arns:
        - !terraform.state image1-ecr global repository_arn
        - !terraform.state image2-ecr global repository_arn
      git_repository: bogus_repo_name
`

	testCases := []struct {
		name        string
		yamlContent string
		description string
	}{
		{
			name:        "Simple list with terraform.output functions",
			yamlContent: yamlContent1,
			description: "Should parse each terraform.output function in the list separately",
		},
		{
			name:        "Mixed list with functions and static values",
			yamlContent: yamlContent2,
			description: "Should correctly handle mix of static values and functions",
		},
		{
			name:        "List with terraform.state functions",
			yamlContent: yamlContent3,
			description: "Should parse each terraform.state function separately (reported issue)",
		},
		{
			name:        "Nested structure with functions in lists",
			yamlContent: yamlContent4,
			description: "Should handle functions in nested list structures",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.description)

			// First, let's see what the raw YAML parsing gives us
			var rawData interface{}
			err := yaml.Unmarshal([]byte(tc.yamlContent), &rawData)
			assert.NoError(t, err, "Raw YAML unmarshaling should succeed")

			t.Logf("Raw YAML data: %+v", rawData)

			// Now test with our custom YAML unmarshaling that processes functions
			result, err := u.UnmarshalYAMLFromFile[map[string]interface{}](atmosConfig, tc.yamlContent, "test.yaml")

			// Log the result and any error for debugging
			if err != nil {
				t.Logf("Error processing YAML with functions: %v", err)
			} else {
				t.Logf("Processed result: %+v", result)
			}

			// For now, we're just checking if it processes without the specific error
			// mentioned in the issue report
			if err != nil {
				// Check if it's the specific error from the issue
				assert.NotContains(t, err.Error(), "invalid number of arguments in the Atmos YAML function",
					"Should not have the 'invalid number of arguments' error that concatenates list items")
			}
		})
	}
}

// TestYamlFunctionsInListsNoConcatenation verifies the fix for the issue where YAML functions
// in lists were being concatenated, causing "invalid number of arguments" errors.
func TestYamlFunctionsInListsNoConcatenation(t *testing.T) {
	// This test specifically addresses the reported issue
	yamlContent := `
components:
  terraform:
    test-component:
      vars:
        # Test case from the reported issue
        ecr_repository_arns:
          - !terraform.state image1-ecr global repository_arn
          - !terraform.state image2-ecr global repository_arn
        # Additional test cases
        outputs_list:
          - !terraform.output component1 stack1 output1
          - !terraform.output component2 stack2 output2
        mixed_list:
          - "static-value"
          - !env ENV_VAR_1
          - !terraform.state component3 stack3 output3
`

	atmosConfig := &schema.AtmosConfiguration{}

	// Parse the YAML
	result, err := u.UnmarshalYAMLFromFile[map[string]interface{}](atmosConfig, yamlContent, "test.yaml")
	assert.NoError(t, err, "Should parse YAML without error")

	// Navigate to the vars section
	components, ok := result["components"].(map[string]interface{})
	assert.True(t, ok, "Should have components section")

	terraform, ok := components["terraform"].(map[string]interface{})
	assert.True(t, ok, "Should have terraform section")

	testComponent, ok := terraform["test-component"].(map[string]interface{})
	assert.True(t, ok, "Should have test-component")

	vars, ok := testComponent["vars"].(map[string]interface{})
	assert.True(t, ok, "Should have vars")

	// Check ecr_repository_arns list - this was the reported issue
	ecrArns, ok := vars["ecr_repository_arns"].([]interface{})
	assert.True(t, ok, "ecr_repository_arns should be a list")
	assert.Equal(t, 2, len(ecrArns), "Should have 2 items in ecr_repository_arns")

	// Verify each item is separate and not concatenated
	for i, arn := range ecrArns {
		arnStr, ok := arn.(string)
		assert.True(t, ok, "List item %d should be a string", i)
		assert.True(t, strings.HasPrefix(arnStr, "!terraform.state"),
			"Item %d should start with !terraform.state", i)

		// CRITICAL: Verify no concatenation happened
		// If concatenation occurred, we'd see multiple !terraform.state in one string
		count := strings.Count(arnStr, "!terraform.state")
		assert.Equal(t, 1, count,
			"Item %d should contain exactly ONE !terraform.state function, not %d (concatenation bug)", i, count)

		// Also verify we don't have multiple components concatenated
		if strings.Contains(arnStr, "image1-ecr") {
			assert.False(t, strings.Contains(arnStr, "image2-ecr"),
				"image1-ecr and image2-ecr should not be in the same string (concatenation bug)")
		}
	}

	// Verify other list types work correctly too
	outputsList, ok := vars["outputs_list"].([]interface{})
	assert.True(t, ok, "outputs_list should be a list")
	assert.Equal(t, 2, len(outputsList), "Should have 2 items in outputs_list")

	for i, output := range outputsList {
		outputStr, ok := output.(string)
		assert.True(t, ok, "List item %d should be a string", i)
		count := strings.Count(outputStr, "!terraform.output")
		assert.Equal(t, 1, count,
			"Item %d should contain exactly ONE !terraform.output function", i)
	}
}
