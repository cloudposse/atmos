package utils

import (
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/list/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// IsNoValuesFoundError checks if an error is a NoValuesFoundError.
func IsNoValuesFoundError(err error) bool {
	_, ok := err.(*errors.NoValuesFoundError)
	return ok
}

// IsEmptyTable checks if the output is an empty table (only contains headers and formatting).
func IsEmptyTable(output string) bool {
	if output == "" {
		return true
	}

	newlineCount := strings.Count(output, "\n")
	if newlineCount <= 4 {
		return true
	}

	return false
}

// CheckComponentExists checks if a component exists in the Atmos configuration.
// It returns true if the component exists, false otherwise.
func CheckComponentExists(atmosConfig *schema.AtmosConfiguration, componentName string) bool {
	if componentName == "" {
		return false
	}

	// Extract component name from path if needed
	parts := strings.Split(componentName, "/")
	baseName := parts[len(parts)-1]

	// Get all stacks to check for the component
	stacksMap, err := e.ExecuteDescribeStacks(*atmosConfig, "", nil, nil, nil, false, false, false, false, nil)
	if err != nil {
		return false
	}

	// Process all stacks to find the component
	for _, stackData := range stacksMap {
		stackMap, ok := stackData.(map[string]interface{})
		if !ok {
			continue
		}

		componentsMap, ok := stackMap["components"].(map[string]interface{})
		if !ok {
			continue
		}

		terraformComponents, ok := componentsMap["terraform"].(map[string]interface{})
		if !ok {
			continue
		}

		// Check if the component exists in this stack
		_, exists := terraformComponents[baseName]
		if exists {
			return true
		}
	}

	return false
}
