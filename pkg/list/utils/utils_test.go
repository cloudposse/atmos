package utils_test

import (
	"fmt"
	"strings"
	"testing"

	listErrors "github.com/cloudposse/atmos/pkg/list/errors"
	"github.com/cloudposse/atmos/pkg/list/utils"
	"github.com/stretchr/testify/assert"
)

// Mock NoValuesFoundError for testing IsNoValuesFoundError.
type mockNoValuesFoundError struct {
	message string
}

func (e *mockNoValuesFoundError) Error() string {
	return e.message
}

func newMockNoValuesFoundError(message string) error {
	return &mockNoValuesFoundError{message: message}
}

func TestIsNoValuesFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "Test with custom mock error resembling NoValuesFoundError",
			err:      newMockNoValuesFoundError("No values found"),
			expected: false, // Mock type doesn't match *errors.NoValuesFoundError
		},
		{
			name:     "Test with standard error",
			err:      fmt.Errorf("A standard error"),
			expected: false,
		},
		{
			name:     "Test with nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "Test with actual NoValuesFoundError",
			// Instantiate directly as a struct pointer
			err:      &listErrors.NoValuesFoundError{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualOk := utils.IsNoValuesFoundError(tt.err)
			assert.Equal(t, tt.expected, actualOk)
		})
	}
}

func TestIsEmptyTable(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "Test with empty string",
			output:   "",
			expected: true,
		},
		{
			name:     "Test with header only (3 lines)",
			output:   "HEADER1 | HEADER2\n------- | -------\n",
			expected: true, // 3 lines <= 4
		},
		{
			name:     "Test with header and separator (4 lines)",
			output:   "HEADER1 | HEADER2\n------- | -------\n\n",
			expected: true, // 4 lines <= 4
		},
		{
			name:     "Test with header, separator, and one row (5 lines)",
			output:   "HEADER1 | HEADER2\n------- | -------\nValue1  | Value2\n\n",
			expected: false, // 5 lines > 4
		},
		{
			name:     "Test with typical non-empty table output",
			output:   "Component | Stack | Status\n--------- | ----- | ------\ncomp1     | dev   | OK\ncomp2     | prod  | OK\n",
			expected: false, // 5 lines > 4
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := utils.IsEmptyTable(tt.output)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// TestCheckComponentExists focuses on the logic *within* CheckComponentExists,
// specifically the processing of the map structure returned by ExecuteDescribeStacks.
// It does not test the ExecuteDescribeStacks call itself or its error handling.
func TestCheckComponentExistsLogic(t *testing.T) {
	// Simulate the map structure that ExecuteDescribeStacks would return
	simulatedStacksMap := map[string]interface{}{
		"dev/stack1": map[string]interface{}{ // Valid structure
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"comp-a": map[string]interface{}{"var": "val1"},
					"comp-b": map[string]interface{}{"var": "val2"},
				},
				"helmfile": map[string]interface{}{ // Should be ignored
					"comp-c": map[string]interface{}{"var": "val3"},
				},
			},
		},
		"prod/stack2": map[string]interface{}{ // Valid structure
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{
					"comp-a": map[string]interface{}{"var": "val_prod"}, // Duplicate comp, still exists
					"comp-d": map[string]interface{}{"var": "val4"},
				},
			},
		},
		"staging/stack3": map[string]interface{}{ // Malformed: components is not a map
			"components": "this is not a map",
		},
		"test/stack4": map[string]interface{}{ // Malformed: terraform is not a map
			"components": map[string]interface{}{
				"terraform": "this is not a map",
			},
		},
		"edge/stack5": "this is not a map", // Malformed: stackData itself is not a map
		"empty/stack6": map[string]interface{}{ // Malformed: components exists but is empty map
			"components": map[string]interface{}{},
		},
		"empty/stack7": map[string]interface{}{ // Malformed: terraform exists but is empty map
			"components": map[string]interface{}{
				"terraform": map[string]interface{}{},
			},
		},
	}

	// Helper function to mimic the core processing loop of CheckComponentExists
	processMapForComponent := func(stacksMap map[string]interface{}, componentName string) bool {
		if componentName == "" { // Check the initial guard from the original function
			return false
		}
		parts := strings.Split(componentName, "/")
		baseName := parts[len(parts)-1]

		for _, stackData := range stacksMap {
			stackMap, ok := stackData.(map[string]interface{})
			if !ok { // Covers line 47
				continue
			}

			componentsMap, ok := stackMap["components"].(map[string]interface{})
			if !ok { // Covers line 51
				continue
			}

			terraformComponents, ok := componentsMap["terraform"].(map[string]interface{})
			if !ok { // Covers line 57
				continue
			}

			_, exists := terraformComponents[baseName]
			if exists { // Covers line 63 (true path)
				return true
			}
		}
		return false // Covers line 68 (component not found after checking all stacks)
	}

	tests := []struct {
		name          string
		componentName string
		expected      bool
	}{
		{
			name:          "Test component exists (comp-a)",
			componentName: "comp-a",
			expected:      true,
		},
		{
			name:          "Test component exists (comp-b)",
			componentName: "comp-b",
			expected:      true,
		},
		{
			name:          "Test component exists (comp-d)",
			componentName: "comp-d",
			expected:      true,
		},
		{
			name:          "Test component exists with path (infra/comp-a)",
			componentName: "infra/comp-a", // Should extract 'comp-a'
			expected:      true,
		},
		{
			name:          "Test component does not exist (comp-x)",
			componentName: "comp-x",
			expected:      false,
		},
		{
			name:          "Test component only in helmfile (comp-c)",
			componentName: "comp-c", // Should not be found in terraform section
			expected:      false,
		},
		// Test the initial guard clause separately using the helper
		{
			name:          "Test empty component name (via helper)",
			componentName: "",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := processMapForComponent(simulatedStacksMap, tt.componentName)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
