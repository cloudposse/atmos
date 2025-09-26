package exec

import (
	"fmt"
	"sync"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/samber/lo"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var componentFuncSyncMap = sync.Map{}

func componentFunc(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
) (any, error) {
	functionName := fmt.Sprintf("atmos.Component(%s, %s)", component, stack)
	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	log.Debug("Executing template function", "function", functionName)

	// If the result for the component in the stack already exists in the cache, return it
	existingSections, found := componentFuncSyncMap.Load(stackSlug)
	if found && existingSections != nil {
		log.Debug("Cache hit for template function", "function", functionName)

		if outputsSection, ok := existingSections.(map[string]any)[cfg.OutputsSectionName]; ok {
			y, err2 := u.ConvertToYAML(outputsSection)
			if err2 != nil {
				log.Error(err2)
			} else {
				log.Debug("'outputs' of the template function", "function", functionName, cfg.OutputsSectionName, y)
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
	componentType := sections[cfg.ComponentTypeSectionName]
	if componentType == cfg.TerraformComponentType {
		// Check if the component in the stack is configured with the 'static' remote state backend,
		// in which case get the `output` from the static remote state instead of executing `terraform output`
		remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(&sections)

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
			cfg.OutputsSectionName: terraformOutputs,
		}

		sections = lo.Assign(sections, outputs)
	}

	// Cache the result
	componentFuncSyncMap.Store(stackSlug, sections)

	log.Debug("Executed template function", "function", functionName)

	// Print the `outputs` section of the Terraform component
	if componentType == cfg.TerraformComponentType {
		y, err2 := u.ConvertToYAML(terraformOutputs)
		if err2 != nil {
			log.Error(err2)
		} else {
			log.Debug("'outputs' of the template function", "function", functionName, cfg.OutputsSectionName, y)
		}
	}

	return sections, nil
}
