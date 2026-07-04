package exec

import (
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

func componentFunc(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	component string,
	stack string,
) (any, error) {
	functionName := fmt.Sprintf("atmos.Component(%s, %s)", component, stack)
	stackSlug := fmt.Sprintf("%s-%s", stack, component)

	log.Debug("Executing template function", "function", functionName)

	// Skip live resolution when the enclosing component is disabled via metadata.enabled.
	// A disabled component has no deployed state; resolving atmos.Component would read remote
	// state and fail. Return empty sections (including an empty `outputs`) so templates that index
	// the result stay nil-safe. Gate on metadata.enabled only, independent of vars.enabled.
	// See docs/fixes/2026-06-22-describe-respect-metadata-enabled.md.
	if enclosingComponentDisabled(configAndStacksInfo) {
		log.Debug("Skipping atmos.Component for disabled enclosing component", "function", functionName)
		return emptyComponentSections(), nil
	}

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
		return nil, errUtils.WrapComponentDescribeError(component, stack, err, "atmos.Component")
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

// enclosingComponentDisabled reports whether the component whose template is being rendered (the
// enclosing component carried by configAndStacksInfo, not the atmos.Component target) is disabled via
// metadata.enabled. A nil info or absent metadata is treated as enabled, so non-describe template
// contexts (e.g. datasource templates built with an empty info) are never affected.
func enclosingComponentDisabled(info *schema.ConfigAndStacksInfo) bool {
	if info == nil {
		return false
	}
	metadata, ok := info.ComponentSection[cfg.MetadataSectionName].(map[string]any)
	if !ok {
		return false
	}
	return !isComponentEnabled(metadata, info.ComponentFromArg)
}

// emptyComponentSections returns the standard component sections as empty maps, including an empty
// outputs map. It is the atmos.Component result for a disabled enclosing component: structurally
// valid (so `.outputs.x`, `.vars.x`, etc. evaluate to nil instead of erroring) while performing no
// describe and no terraform state/output read.
func emptyComponentSections() map[string]any {
	return map[string]any{
		cfg.VarsSectionName:      map[string]any{},
		cfg.SettingsSectionName:  map[string]any{},
		cfg.EnvSectionName:       map[string]any{},
		cfg.MetadataSectionName:  map[string]any{},
		cfg.ProvidersSectionName: map[string]any{},
		cfg.HooksSectionName:     map[string]any{},
		cfg.OverridesSectionName: map[string]any{},
		cfg.BackendSectionName:   map[string]any{},
		cfg.OutputsSectionName:   map[string]any{},
	}
}
