package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components
func ExecuteDescribeStacks(
	cliConfig schema.CliConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	includeEmptyStacks bool,
) (map[string]any, error) {

	return e.ExecuteDescribeStacks(cliConfig, filterByStack, components, componentTypes, sections, ignoreMissingFiles, false, includeEmptyStacks)
}
