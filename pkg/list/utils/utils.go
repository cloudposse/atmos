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
	return newlineCount <= 4
}

// CheckComponentExists checks if a component exists in the Atmos configuration.
// It returns true if the component exists, false otherwise.
func CheckComponentExists(atmosConfig *schema.AtmosConfiguration, componentName string) bool {
	if componentName == "" {
		return false
	}

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

		for _, componentTypeMap := range componentsMap {
			typedComponents, ok := componentTypeMap.(map[string]interface{})
			if !ok {
				continue
			}

			_, exists := typedComponents[componentName]
			if exists {
				return true
			}
		}
	}

	return false
}
