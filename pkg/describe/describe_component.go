package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
)

// ExecuteDescribeComponent describes component config and returns the final map of component configuration in the stack
func ExecuteDescribeComponent(
	component string,
	stack string,
	processTemplates bool,
) (map[string]any, error) {
	return e.ExecuteDescribeComponent(component, stack, processTemplates)
}
