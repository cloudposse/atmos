package exec

import (
	"fmt"
	"sync"

	log "github.com/charmbracelet/log"
	"github.com/samber/lo"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var componentFuncSyncMap = sync.Map{}

func componentFunc(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	component string,
	stack string,
) (any, error) {
	functionName := fmt.Sprintf("atmos.Component(%s, %s)", component, stack)
	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	log.Debug("Executing template function", "function", functionName)

	// If the result for the component in the stack already exists in the cache, return it
	existingSections, found := componentFuncSyncMap.Load(stackSlug)
	if found && existingSections != nil {
		log.Debug("cache hit for template function", "function", functionName)

		if outputsSection, ok := existingSections.(map[string]any)["outputs"]; ok {
			y, err2 := u.ConvertToYAML(outputsSection)
			if err2 != nil {
				log.Error(err2)
			} else {
				log.Debug("Result of the template function", "function", functionName, "outputs", y)
			}
		}

		return existingSections, nil
	}

	sections, err := ExecuteDescribeComponent(component, stack, true, true, nil)
	if err != nil {
		return nil, err
	}

	// Process Terraform remote state
	var terraformOutputs map[string]any
	if configAndStacksInfo.ComponentType == cfg.TerraformComponentType {
		// Check if the component in the stack is configured with the 'static' remote state backend,
		// in which case get the `output` from the static remote state instead of executing `terraform output`
		remoteStateBackendStaticTypeOutputs, err := GetComponentRemoteStateBackendStaticType(sections)
		if err != nil {
			return nil, err
		}

		if remoteStateBackendStaticTypeOutputs != nil {
			// Return the static backend outputs
			terraformOutputs = remoteStateBackendStaticTypeOutputs
		} else {
			// Execute `terraform output`
			terraformOutputs, err = execTerraformOutput(atmosConfig, component, stack, sections)
			if err != nil {
				return nil, err
			}
		}

		outputs := map[string]any{
			"outputs": terraformOutputs,
		}

		sections = lo.Assign(sections, outputs)
	}

	// Cache the result
	componentFuncSyncMap.Store(stackSlug, sections)

	log.Debug("Executed template function", "function", functionName)

	if configAndStacksInfo.ComponentType == cfg.TerraformComponentType {
		y, err2 := u.ConvertToYAML(terraformOutputs)
		if err2 != nil {
			log.Error(err2)
		} else {
			log.Debug("Result of the template function", "function", functionName, "outputs", y)
		}
	}

	return sections, nil
}
