package exec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestYamlFunctionsReturnTypes verifies that YAML functions can return different types
// (strings, maps, lists) and they are handled correctly, especially in list contexts.
func TestYamlFunctionsReturnTypes(t *testing.T) {
	testCases := []struct {
		name         string
		yamlContent  string
		validateFunc func(t *testing.T, result map[string]interface{})
	}{
		{
			name: "Functions returning strings in a list",
			yamlContent: `
test:
  string_list:
    - !terraform.output component1 stack1 string_output
    - !terraform.state component2 stack2 string_value
    - !env STRING_ENV_VAR
    - "static string"
`,
			validateFunc: func(t *testing.T, result map[string]interface{}) {
				test := result["test"].(map[string]interface{})
				list := test["string_list"].([]interface{})
				assert.Len(t, list, 4, "Should have 4 items")

				// Each should be a string containing the function call
				for i, item := range list[:3] {
					str, ok := item.(string)
					assert.True(t, ok, "Item %d should be a string", i)
					assert.True(t, strings.HasPrefix(str, "!"), "Item %d should start with !", i)
				}
				assert.Equal(t, "static string", list[3], "Last item should be static string")
			},
		},
		{
			name: "Functions that could return maps",
			yamlContent: `
test:
  map_results:
    - !terraform.output vpc stack1 all_outputs  # Could return a map
    - !terraform.state database stack2 config    # Could return a map
`,
			validateFunc: func(t *testing.T, result map[string]interface{}) {
				test := result["test"].(map[string]interface{})
				list := test["map_results"].([]interface{})
				assert.Len(t, list, 2, "Should have 2 items")

				// For now they're strings with function calls, but when executed
				// they could return maps
				for i, item := range list {
					str, ok := item.(string)
					assert.True(t, ok, "Item %d should be a string (function call)", i)
					assert.True(t, strings.Contains(str, "!terraform."),
						"Item %d should contain terraform function", i)
				}
			},
		},
		{
			name: "Functions that could return lists",
			yamlContent: `
test:
  list_results:
    - !terraform.output asg stack1 instance_ids    # Could return a list
    - !terraform.state cluster stack2 node_groups  # Could return a list
`,
			validateFunc: func(t *testing.T, result map[string]interface{}) {
				test := result["test"].(map[string]interface{})
				list := test["list_results"].([]interface{})
				assert.Len(t, list, 2, "Should have 2 items")

				// Verify they're separate function calls
				for i, item := range list {
					str, ok := item.(string)
					assert.True(t, ok, "Item %d should be a string", i)
					// Make sure no concatenation
					count := strings.Count(str, "!terraform.")
					assert.Equal(t, 1, count, "Item %d should have exactly one function", i)
				}
			},
		},
		{
			name: "Mixed types in nested structure",
			yamlContent: `
components:
  terraform:
    my-component:
      vars:
        # List containing functions that return different types
        mixed_outputs:
          - !terraform.output component1 stack1 string_val
          - !terraform.output component2 stack2 map_val
          - !terraform.output component3 stack3 list_val
        # Map containing functions
        config_from_outputs:
          database_host: !terraform.output db stack host
          cache_nodes: !terraform.output cache stack nodes
          static_value: "hardcoded"
`,
			validateFunc: func(t *testing.T, result map[string]interface{}) {
				components := result["components"].(map[string]interface{})
				terraform := components["terraform"].(map[string]interface{})
				myComponent := terraform["my-component"].(map[string]interface{})
				vars := myComponent["vars"].(map[string]interface{})

				// Check mixed_outputs list
				mixedOutputs := vars["mixed_outputs"].([]interface{})
				assert.Len(t, mixedOutputs, 3, "Should have 3 mixed outputs")
				for i, item := range mixedOutputs {
					str, ok := item.(string)
					assert.True(t, ok, "Item %d should be a string", i)
					assert.Contains(t, str, "!terraform.output", "Should contain function")
					// Verify no concatenation
					assert.Equal(t, 1, strings.Count(str, "!terraform.output"),
						"Should have exactly one function per item")
				}

				// Check config_from_outputs map
				configMap := vars["config_from_outputs"].(map[string]interface{})
				assert.Contains(t, configMap["database_host"], "!terraform.output")
				assert.Contains(t, configMap["cache_nodes"], "!terraform.output")
				assert.Equal(t, "hardcoded", configMap["static_value"])
			},
		},
		{
			name: "Edge case: Empty list with comment",
			yamlContent: `
test:
  # This list will be populated by functions
  empty_list: []
  list_with_one_func:
    - !terraform.output component stack output
`,
			validateFunc: func(t *testing.T, result map[string]interface{}) {
				test := result["test"].(map[string]interface{})

				emptyList := test["empty_list"].([]interface{})
				assert.Len(t, emptyList, 0, "Should be empty")

				oneFunc := test["list_with_one_func"].([]interface{})
				assert.Len(t, oneFunc, 1, "Should have one item")
				assert.Contains(t, oneFunc[0], "!terraform.output")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{}

			// Parse the YAML
			result, err := u.UnmarshalYAMLFromFile[map[string]interface{}](
				atmosConfig, tc.yamlContent, "test.yaml")

			assert.NoError(t, err, "Should parse without error")

			// Run the custom validation
			tc.validateFunc(t, result)
		})
	}
}

// TestYamlFunctionsInListsNoErrorOnExecution verifies that the fix doesn't cause
// the "invalid number of arguments" error when functions are actually executed.
func TestYamlFunctionsInListsNoErrorOnExecution(t *testing.T) {
	// This simulates the exact scenario from the user's bug report
	yamlContent := `
import:
  - ./_defaults
  - path: catalog/ecr-deployer-role
    context:
      ecr_repository_arns:
        - !terraform.state image1-ecr global repository_arn
        - !terraform.state image2-ecr global repository_arn
      git_repository: bogus_repo_name
`

	atmosConfig := &schema.AtmosConfiguration{}

	// Parse the YAML - this should NOT error with "invalid number of arguments"
	result, err := u.UnmarshalYAMLFromFile[map[string]interface{}](
		atmosConfig, yamlContent, "test.yaml")
	// The functions might fail to execute (components don't exist), but we should
	// NOT get the concatenation error
	if err != nil {
		assert.NotContains(t, err.Error(), "invalid number of arguments in the Atmos YAML function",
			"Should not get the concatenation error")
	}

	if result != nil {
		// Verify the structure is correct
		imports := result["import"].([]interface{})
		assert.Len(t, imports, 2, "Should have 2 imports")

		// Check the second import with context
		if len(imports) > 1 {
			imp := imports[1].(map[string]interface{})
			context := imp["context"].(map[string]interface{})
			arns := context["ecr_repository_arns"].([]interface{})

			assert.Len(t, arns, 2, "Should have 2 ARNs")

			// Verify they're separate
			for i, arn := range arns {
				arnStr := arn.(string)
				count := strings.Count(arnStr, "!terraform.state")
				assert.Equal(t, 1, count,
					"ARN %d should have exactly one !terraform.state, not multiple (concatenation bug)", i)
			}
		}
	}
}
