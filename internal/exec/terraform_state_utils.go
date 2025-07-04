package exec

import (
	"fmt"
	"sync"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

var terraformStateCache = sync.Map{}

func GetTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	yamlFunc string,
	stack string,
	component string,
	output string,
	skipCache bool,
) (any, error) {
	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	if !skipCache {
		backend, found := terraformStateCache.Load(stackSlug)
		if found && backend != nil {
			log.Debug("Cache hit",
				"function", yamlFunc,
				cfg.ComponentStr, component,
				cfg.StackStr, stack,
				"output", output,
			)
			result, err := GetTerraformBackendVariable(atmosConfig, backend.(map[string]any), output)
			if err != nil {
				er := fmt.Errorf("%w %s for component `%s` in stack `%s` in YAML function `%s`. Error: %v", errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err)
				return nil, er
			}
			return result, nil
		}
	}

	componentSections, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		er := fmt.Errorf("%w `%s` in stack `%s` in YAML function `%s`. Error: %v", errUtils.ErrDescribeComponent, component, stack, yamlFunc, err)
		return nil, er
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(componentSections)

	// Read static remote state backend outputs
	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformStateCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		result := getStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
		return result, nil
	}

	// Read Terraform backend
	backend, err := GetTerraformBackend(atmosConfig, componentSections)
	if err != nil {
		er := fmt.Errorf("%w for component `%s` in stack `%s` in YAML function `%s`.\nerror: %v", errUtils.ErrReadTerraformBackend, component, stack, yamlFunc, err)
		return nil, er
	}

	// Cache the result
	terraformStateCache.Store(stackSlug, backend)

	// Get the output
	result, err := GetTerraformBackendVariable(atmosConfig, backend, output)
	if err != nil {
		er := fmt.Errorf("%w %s for component `%s` in stack `%s` in YAML function `%s`.\nerror: %v", errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err)
		return nil, er
	}
	return result, nil
}
