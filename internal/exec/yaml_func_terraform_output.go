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

	parts := strings.Split(str, " ")

	if len(parts) != 3 {
		err := errors.New(fmt.Sprintf("invalid Atmos YAML function: %s\nthree parameters are required: component, stack, output", input))
		u.LogErrorAndExit(cliConfig, err)
	}

	component := strings.TrimSpace(parts[0])
	stack := strings.TrimSpace(parts[1])
	output := strings.TrimSpace(parts[2])

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

	outputProcessed, err := execTerraformOutput(cliConfig, component, stack, sections)
	if err != nil {
		u.LogErrorAndExit(cliConfig, err)
	}

	// Cache the result
	terraformOutputFuncSyncMap.Store(stackSlug, outputProcessed)

	return getTerraformOutput(cliConfig, input, component, stack, outputProcessed, output)
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
