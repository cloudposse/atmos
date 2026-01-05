package exec

import (
	"errors"
	"fmt"
	"sync"

	"github.com/samber/lo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var componentFuncSyncMap = sync.Map{}

// wrapComponentFuncError wraps an error from ExecuteDescribeComponent, breaking the
// ErrInvalidComponent chain to prevent triggering component type fallback.
func wrapComponentFuncError(component, stack string, err error) error {
	if errors.Is(err, errUtils.ErrInvalidComponent) {
		// Break the ErrInvalidComponent chain by using ErrDescribeComponent as the base.
		// This ensures that errors from template function processing don't trigger
		// fallback to try other component types.
		return fmt.Errorf("%w: atmos.Component(%s, %s): %s",
			errUtils.ErrDescribeComponent, component, stack, err.Error())
	}
	return fmt.Errorf("atmos.Component(%s, %s) failed: %w", component, stack, err)
}

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

	// Create AuthManager wrapper from configAndStacksInfo to propagate auth context.
	var authMgr auth.AuthManager
	if configAndStacksInfo != nil && configAndStacksInfo.AuthContext != nil {
		authMgr = newAuthContextWrapper(configAndStacksInfo.AuthContext)
	}

	sections, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          authMgr,
	})
	if err != nil {
		return nil, wrapComponentFuncError(component, stack, err)
	}

	// Process Terraform remote state.
	var terraformOutputs map[string]any
	componentType := sections[cfg.ComponentTypeSectionName]
	if componentType == cfg.TerraformComponentType {
		// Check if the component in the stack is configured with the 'static' remote state backend,
		// in which case get the `output` from the static remote state instead of executing `terraform output`.
		remoteStateBackendStaticTypeOutputs := GetComponentRemoteStateBackendStaticType(&sections)

		if remoteStateBackendStaticTypeOutputs != nil {
			// Return the static backend outputs.
			terraformOutputs = remoteStateBackendStaticTypeOutputs
		} else {
			// Execute `terraform output` with authContext from configAndStacksInfo (populated by --identity flag).
			var authContext *schema.AuthContext
			if configAndStacksInfo != nil {
				authContext = configAndStacksInfo.AuthContext
			}
			terraformOutputs, err = tfoutput.ExecuteWithSections(atmosConfig, component, stack, sections, authContext)
			if err != nil {
				return nil, fmt.Errorf("atmos.Component(%s, %s) failed to get terraform outputs: %w", component, stack, err)
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
