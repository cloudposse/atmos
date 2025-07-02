package exec

import (
	"fmt"
	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
		cachedOutputs, found := terraformOutputsCache.Load(stackSlug)
		if found && cachedOutputs != nil {
			log.Debug("Cache hit for terraform output",
				"command", fmt.Sprintf("!terraform.output %s %s %s", component, stack, output),
				cfg.ComponentStr, component,
				cfg.StackStr, stack,
				"output", output,
			)
			return getTerraformOutputVariable(atmosConfig, component, stack, cachedOutputs.(map[string]any), output)
		}
	}

	message := fmt.Sprintf("Fetching %s output from %s in %s", output, component, stack)

	if atmosConfig.Logs.Level == u.LogLevelTrace || atmosConfig.Logs.Level == u.LogLevelDebug {
		// Initialize spinner
		p := NewSpinner(message)
		spinnerDone := make(chan struct{})
		// Run spinner in a goroutine
		RunSpinner(p, spinnerDone, message)
		// Ensure the spinner is stopped before returning
		defer StopSpinner(p, spinnerDone)
	}

	sections, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		u.PrintfMessageToTUI("\r✗ %s\n", message)
		er := fmt.Errorf("failed to describe the component %s in the stack %s. Error: %w", component, stack, err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	// Check if the component in the stack is configured with the 'static' remote state backend, in which case get the
	// `output` from the static remote state instead of executing `terraform output`
	remoteStateBackendStaticTypeOutputs, err := GetComponentRemoteStateBackendStaticType(sections)
	if err != nil {
		u.PrintfMessageToTUI("\r✗ %s\n", message)
		er := fmt.Errorf("failed to get static remote state backend outputs. Error: %w", err)
		errUtils.CheckErrorPrintAndExit(er, "", "")
	}

	var result any
	if remoteStateBackendStaticTypeOutputs != nil {
		// Cache the result
		terraformOutputsCache.Store(stackSlug, remoteStateBackendStaticTypeOutputs)
		result = getStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateBackendStaticTypeOutputs, output)
	} else {
		// Execute `terraform output`
		terraformOutputs, err := execTerraformOutput(atmosConfig, component, stack, sections)
		if err != nil {
			u.PrintfMessageToTUI("\r✗ %s\n", message)
			er := fmt.Errorf("failed to execute terraform output for the component %s in the stack %s. Error: %w", component, stack, err)
			errUtils.CheckErrorPrintAndExit(er, "", "")
		}

		// Cache the result
		terraformOutputsCache.Store(stackSlug, terraformOutputs)
		result = getTerraformOutputVariable(atmosConfig, component, stack, terraformOutputs, output)
	}
	u.PrintfMessageToTUI("\r✓ %s\n", message)

	return result
}
