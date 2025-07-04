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
	stack string,
	component string,
	output string,
	skipCache bool,
) any {
	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	if !skipCache {
		backend, found := terraformStateCache.Load(stackSlug)
		if found && backend != nil {
			log.Debug("Cache hit",
				"function", fmt.Sprintf("!terraform.state %s %s %s", component, stack, output),
				cfg.ComponentStr, component,
				cfg.StackStr, stack,
				"output", output,
			)
			return GetTerraformBackendVariable(atmosConfig, component, stack, backend.(map[string]any), output)
		}
	}

	sections, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		er := fmt.Errorf("failed to describe the component %s in the stack %s. Error: %w", component, stack, err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(sections)

	// Read static remote state backend outputs
	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformStateCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		result := getStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
		return result
	}

	// Read Terraform backend
	backend, err := GetTerraformBackend(atmosConfig, component, stack, sections)
	if err != nil {
		er := fmt.Errorf("failed to get terraform state for the component %s in the stack %s. Error: %w", component, stack, err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Cache the result
	terraformStateCache.Store(stackSlug, backend)

	// Get the output
	result := GetTerraformBackendVariable(atmosConfig, component, stack, backend, output)
	return result
}
