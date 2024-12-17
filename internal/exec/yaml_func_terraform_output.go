package exec

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var (
	terraformOutputFuncSyncMap = sync.Map{}
)

func processTagTerraformOutput(
	cliConfig schema.CliConfiguration,
	input string,
	currentStack string,
) any {
	u.LogTrace(cliConfig, fmt.Sprintf("Executing Atmos YAML function: %s", input))

	str, err := getStringAfterTag(cliConfig, input, config.AtmosYamlFuncTerraformOutput)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	var component string
	var stack string
	var output string

	// Split the string into slices based on any whitespace (one or more spaces, tabs, or newlines),
	// while also ignoring leading and trailing whitespace
	parts := strings.Fields(str)
	partsLen := len(parts)

	if partsLen == 3 {
		component = strings.TrimSpace(parts[0])
		stack = strings.TrimSpace(parts[1])
		output = strings.TrimSpace(parts[2])
	} else if partsLen == 2 {
		component = strings.TrimSpace(parts[0])
		stack = currentStack
		output = strings.TrimSpace(parts[1])
		u.LogTrace(cliConfig, fmt.Sprintf("Atmos YAML function `%s` is called with two parameters 'component' and 'output'. "+
			"Using the current stack '%s' as the 'stack' parameter", input, currentStack))
	} else {
		err := errors.New(fmt.Sprintf("invalid number of arguments in the Atmos YAML function: %s", input))
		u.LogErrorAndExit(cliConfig, err)
	}

	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	// If the result for the component in the stack already exists in the cache, return it
	cachedOutputs, found := terraformOutputFuncSyncMap.Load(stackSlug)
	if found && cachedOutputs != nil {
		u.LogTrace(cliConfig, fmt.Sprintf("Found the result of the Atmos YAML function '!terraform.output %s %s %s' in the cache", component, stack, output))
		return getTerraformOutput(cliConfig, input, component, stack, cachedOutputs.(map[string]any), output)
	}

	sections, err := ExecuteDescribeComponent(component, stack, true)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	// Check if the component in the stack is configured with the 'static' remote state backend,
	// in which case get the `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs, err := GetComponentRemoteStateBackendStaticType(sections)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformOutputFuncSyncMap.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		return getStaticRemoteStateOutput(cliConfig, input, component, stack, remoteStateBackendStaticTypeOutputs, output)
	} else {
		// Execute `terraform output`
		terraformOutputs, err := execTerraformOutput(cliConfig, component, stack, sections)
		if err != nil {
			u.LogErrorAndExit(cliConfig, err)
		}

		// Cache the result
		terraformOutputFuncSyncMap.Store(stackSlug, terraformOutputs)
		return getTerraformOutput(cliConfig, input, component, stack, terraformOutputs, output)
	}
}

func getTerraformOutput(
	cliConfig schema.CliConfiguration,
	funcDef string,
	component string,
	stack string,
	outputs map[string]any,
	output string,
) any {
	if u.MapKeyExists(outputs, output) {
		return outputs[output]
	}

	u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' does not have the output '%s'",
		funcDef,
		component,
		stack,
		output,
	))

	return nil
}

func getStaticRemoteStateOutput(
	cliConfig schema.CliConfiguration,
	funcDef string,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) any {
	if u.MapKeyExists(remoteStateSection, output) {
		return remoteStateSection[output]
	}

	u.LogErrorAndExit(cliConfig, fmt.Errorf("invalid Atmos YAML function: %s\nthe component '%s' in the stack '%s' "+
		"is configured with the 'static' remote state backend, but the remote state backend does not have the output '%s'",
		funcDef,
		component,
		stack,
		output,
	))

	return nil
}
