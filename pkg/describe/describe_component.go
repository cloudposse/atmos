package describe

import (
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ExecuteDescribeComponent describes component config and returns the final map of component configuration in the stack
func ExecuteDescribeComponent(
	component string,
	stack string,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
) (map[string]any, error) {
	defer perf.Track(nil, "describe.ExecuteDescribeComponent")()

	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     processTemplates,
		ProcessYamlFunctions: processYamlFunctions,
		Skip:                 skip,
		AuthManager:          nil,
	})
}
