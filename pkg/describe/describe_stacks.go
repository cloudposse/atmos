package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components
func ExecuteDescribeStacks(
	atmosConfig schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	includeEmptyStacks bool,
	processTemplates bool,
	processYamlFunctions bool,
) (map[string]any, error) {
	return e.ExecuteDescribeStacks(atmosConfig, filterByStack, components, componentTypes, sections, ignoreMissingFiles, processTemplates, processYamlFunctions, includeEmptyStacks, nil)
}
