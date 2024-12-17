package exec

import (
	"fmt"
	"sync"

	"github.com/samber/lo"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	componentFuncSyncMap = sync.Map{}
)

func componentFunc(cliConfig schema.CliConfiguration, component string, stack string) (any, error) {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing template function 'atmos.Component(%s, %s)'", component, stack))

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	existingSections, found := componentFuncSyncMap.Load(stackSlug)
	if found && existingSections != nil {
		if cliConfig.Logs.Level == u.LogLevelTrace {
			u.LogTrace(cliConfig, fmt.Sprintf("Found the result of the template function 'atmos.Component(%s, %s)' in the cache", component, stack))

			if outputsSection, ok := existingSections.(map[string]any)["outputs"]; ok {
				u.LogTrace(cliConfig, "'outputs' section:")
				y, err2 := u.ConvertToYAML(outputsSection)
				if err2 != nil {
					u.LogError(cliConfig, err2)
				} else {
					u.LogTrace(cliConfig, y)
				}
			}
		}

		return existingSections, nil
	}

	sections, err := ExecuteDescribeComponent(component, stack, true)
	if err != nil {
		return nil, err
	}

	var terraformOutputs map[string]any

	// Check if the component in the stack is configured with the 'static' remote state backend,
	// in which case get the `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs, err := GetComponentRemoteStateBackendStaticType(sections)
	if err != nil {
		return nil, err
	}

	if remoteStateBackendStaticTypeOutputs != nil {
		terraformOutputs = remoteStateBackendStaticTypeOutputs
	} else {
		// Execute `terraform output`
		terraformOutputs, err = execTerraformOutput(cliConfig, component, stack, sections)
		if err != nil {
			return nil, err
		}
	}

	outputs := map[string]any{
		"outputs": terraformOutputs,
	}

	sections = lo.Assign(sections, outputs)

	// Cache the result
	componentFuncSyncMap.Store(stackSlug, sections)

	if cliConfig.Logs.Level == u.LogLevelTrace {
		u.LogTrace(cliConfig, fmt.Sprintf("Executed template function 'atmos.Component(%s, %s)'\n\n'outputs' section:", component, stack))
		y, err2 := u.ConvertToYAML(terraformOutputs)
		if err2 != nil {
			u.LogError(cliConfig, err2)
		} else {
			u.LogTrace(cliConfig, y)
		}
	}

	return sections, nil
}
