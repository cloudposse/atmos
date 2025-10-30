package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteDescribeStacks processes stack manifests and returns the final map of stacks and components.
func ExecuteDescribeStacks(
	atmosConfig schema.AtmosConfiguration,
	filterByStack string,
	components []string,
	componentTypes []string,
	sections []string,
	ignoreMissingFiles bool,
	includeEmptyStacks bool,
) (map[string]any, error) {
	defer perf.Track(&atmosConfig, "describe.ExecuteDescribeStacks")()

	return e.ExecuteDescribeStacks(
		&atmosConfig,
		filterByStack,
		components,
		componentTypes,
		sections,
		ignoreMissingFiles,
		true,
		true,
		includeEmptyStacks,
		nil,
	)
}
