package exec

import (
	"fmt"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"

	log "github.com/cloudposse/atmos/pkg/logger"

	errUtils "github.com/cloudposse/atmos/errors"
	tb "github.com/cloudposse/atmos/internal/terraform_backend"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

var terraformStateCache = sync.Map{}

// GetTerraformState retrieves a specified Terraform output variable for a given component within a stack.
// It optionally uses a cache to avoid redundant state retrievals and supports both static and dynamic backends.
// Parameters:
//   - atmosConfig: Atmos configuration pointer
//   - yamlFunc: Name of the calling YAML function for error context
//   - stack: Stack identifier
//   - component: Component identifier
//   - output: Output variable key to retrieve
//   - skipCache: Flag to bypass cache lookup
//   - authContext: Optional auth context containing Atmos-managed credentials
//
// Returns the output value or nil if the component is not provisioned.
func GetTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	yamlFunc string,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
) (any, error) {
	defer perf.Track(atmosConfig, "exec.GetTerraformState")()

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it.
	if !skipCache {
		backend, found := terraformStateCache.Load(stackSlug)
		if found && backend != nil {
			log.Debug("Cache hit",
				"function", yamlFunc,
				cfg.ComponentStr, component,
				cfg.StackStr, stack,
				"output", output,
			)
			result, err := tb.GetTerraformBackendVariable(atmosConfig, backend.(map[string]any), output)
			if err != nil {
				er := fmt.Errorf("%w %s for component `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err)
				return nil, er
			}
			return result, nil
		}
	}

	componentSections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
	if err != nil {
		er := fmt.Errorf("%w `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrDescribeComponent, component, stack, yamlFunc, err)
		return nil, er
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`.
	remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(&componentSections)

	// Read static remote state backend outputs.
	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformStateCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		result, exists, err := GetStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
		if err != nil {
			return nil, fmt.Errorf("%w for component `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrReadTerraformState, component, stack, yamlFunc, err)
		}
		if !exists {
			return nil, fmt.Errorf("%w: output `%s` does not exist for component `%s` in stack `%s`\nin YAML function: `%s`", errUtils.ErrReadTerraformState, output, component, stack, yamlFunc)
		}
		// result may be nil if the output is legitimately null
		return result, nil
	}

	// Read Terraform backend.
	backend, err := tb.GetTerraformBackend(atmosConfig, &componentSections, authContext)
	if err != nil {
		er := fmt.Errorf("%w for component `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrReadTerraformState, component, stack, yamlFunc, err)
		return nil, er
	}

	// Cache the result.
	terraformStateCache.Store(stackSlug, backend)

	// If `backend` is `nil`, return `nil` (the component in the stack has not been provisioned yet).
	if backend == nil {
		return nil, nil
	}

	// Get the output.
	result, err := tb.GetTerraformBackendVariable(atmosConfig, backend, output)
	if err != nil {
		er := fmt.Errorf("%w %s for component `%s` in stack `%s`\nin YAML function: `%s`\n%v", errUtils.ErrEvaluateTerraformBackendVariable, output, component, stack, yamlFunc, err)
		return nil, er
	}

	return result, nil
}
