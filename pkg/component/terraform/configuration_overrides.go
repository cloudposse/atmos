package terraform

import (
	"fmt"
	"maps"

	errUtils "github.com/cloudposse/atmos/errors"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A component owned by a configuration provider (for example a Terragrunt unit imported or
// discovered by the Terragrunt plugin) is configured by two layers. The provider's own evaluation
// is the base; everything the stack manifests say about the component deep-merges over it with
// the usual rules, so any setting can be overridden from stack configuration. From lowest to
// highest precedence, per section:
//
//	vars       provider inputs < stack vars (imports, catalog, inherits, the component, overrides)
//	env        provider extra_arguments environment < stack env
//	settings   stack settings (the provider only marks ownership under settings.configuration)
//	metadata   stack metadata (terraform_workspace becomes TF_WORKSPACE for the evaluation)
//	command    stack command selects the binary the provider evaluates and runs
//	hooks, providers, auth, dependencies, locals: stack configuration only
//
// Two settings are owned by the provider and cannot be overridden, because other readers depend
// on them being the same everywhere: the state backend (dependency outputs, !terraform.output and
// saved-plan verification all resolve it from the unit) and the module (metadata.component).
// Trying to override them is an error, never a silent ignore.

// localBackendType is the placeholder backend type the provider gives every discovered component.
const localBackendType = "local"

// checkStackOverrides rejects stack configuration that tries to change a provider-owned setting.
func checkStackOverrides(info *schema.ConfigAndStacksInfo) error {
	if setting, overridden := backendOverride(info); overridden {
		return overrideError(info, setting, "the state backend",
			"Define the backend in the unit's `remote_state` block (or its `generate \"backend\"` block); the unit's state identity is shared with dependency reads and saved plans.")
	}
	if component, _ := info.ComponentMetadataSection["component"].(string); component != "" {
		return overrideError(info, "metadata.component: "+component, "the Terraform module",
			"The module comes from the unit's `terraform { source = ... }`. To use another module, change the unit, or define a regular component that sets `metadata.component`.")
	}
	return nil
}

// backendOverride reports a backend the stack defines for the component itself. A backend that is
// only inherited from a global section never reaches here, because the provider's placeholder is
// part of the component and takes precedence over inherited values.
func backendOverride(info *schema.ConfigAndStacksInfo) (string, bool) {
	if kind, _ := info.ComponentSection["backend_type"].(string); kind != "" && kind != localBackendType {
		return "backend_type: " + kind, true
	}
	backend, _ := info.ComponentSection["backend"].(map[string]any)
	if local, _ := backend[localBackendType].(map[string]any); len(local) > 0 {
		return "backend." + localBackendType, true
	}
	return "", false
}

func overrideError(info *schema.ConfigAndStacksInfo, setting, owned, hint string) error {
	return errUtils.Build(fmt.Errorf("%w: %s of component %q in stack %q is owned by the configuration provider, so `%s` cannot be set", errUtils.ErrConfigurationOverrideUnsupported, owned, info.ComponentFromArg, info.Stack, setting)).
		WithContext("component", info.ComponentFromArg).
		WithContext("stack", info.Stack).
		WithContext("setting", setting).
		WithHint(hint).
		Err()
}

// overlayStackVars deep-merges the stack's vars over the provider's inputs. The provider
// reevaluates its inputs for every operation, so the result is computed here, per operation, and
// neither side is stored.
func overlayStackVars(config *schema.AtmosConfiguration, inputs, stackVars map[string]any) (map[string]any, error) {
	if len(stackVars) == 0 {
		return inputs, nil
	}
	merged, err := m.Merge(config, []map[string]any{inputs, stackVars})
	if err != nil {
		return nil, fmt.Errorf("component vars cannot be merged over the provider inputs: %w", err)
	}
	return merged, nil
}

// withoutStackEnvironment drops provider environment entries that the stack's env section also
// sets, so the stack wins. The provider's execution data is copied, not modified in place.
func withoutStackEnvironment(execution *schema.ComponentExecution, stackEnv map[string]any) *schema.ComponentExecution {
	if execution == nil || len(stackEnv) == 0 {
		return execution
	}
	result := *execution
	result.ExtraArguments = make([]schema.ComponentExtraArguments, len(execution.ExtraArguments))
	for i, item := range execution.ExtraArguments {
		result.ExtraArguments[i] = item
		if len(item.Environment) == 0 {
			continue
		}
		kept := maps.Clone(item.Environment)
		for key := range stackEnv {
			delete(kept, key)
		}
		result.ExtraArguments[i].Environment = kept
	}
	return &result
}

// applyStackOverrides layers the stack's vars and env over what the provider prepared. A reviewed
// saved plan already embodies its inputs, so only the environment is layered for it.
func applyStackOverrides(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) error {
	prepared.Execution = withoutStackEnvironment(prepared.Execution, info.ComponentEnvSection)
	if usesConfigurationPlan(info) {
		return nil
	}
	inputs, err := overlayStackVars(config, prepared.Inputs, info.ComponentVarsSection)
	if err != nil {
		return err
	}
	prepared.Inputs = inputs
	return nil
}
