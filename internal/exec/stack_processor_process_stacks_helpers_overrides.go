package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// processComponentOverrides processes component overrides sections.
//
//nolint:gocognit,revive,cyclop,funlen // Processes multiple override sections with type-specific validation.
func processComponentOverrides(opts *ComponentProcessorOptions, result *ComponentProcessorResult) error {
	defer perf.Track(opts.AtmosConfig, "exec.processComponentOverrides")()

	// Initialize overrides.
	result.ComponentOverridesVars = make(map[string]any)
	result.ComponentOverridesSettings = make(map[string]any)
	result.ComponentOverridesEnv = make(map[string]any)
	result.ComponentOverridesAuth = make(map[string]any)
	if opts.ComponentType == cfg.TerraformComponentType {
		result.ComponentOverridesProviders = make(map[string]any)
		result.ComponentOverridesHooks = make(map[string]any)
	}

	i, ok := opts.ComponentMap[cfg.OverridesSectionName]
	if !ok {
		result.ComponentOverrides = make(map[string]any)
		return nil
	}

	componentOverrides, ok := i.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: 'components.%s.%s.overrides' in the manifest '%s'", errUtils.ErrInvalidComponentOverrides, opts.ComponentType, opts.Component, opts.StackName)
	}
	result.ComponentOverrides = componentOverrides

	// Extract vars overrides.
	if i, ok := componentOverrides[cfg.VarsSectionName]; ok {
		componentOverridesVars, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.overrides.vars' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesVars, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverridesVars = componentOverridesVars
	}

	// Extract settings overrides.
	if i, ok := componentOverrides[cfg.SettingsSectionName]; ok {
		componentOverridesSettings, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.overrides.settings' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesSettings, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverridesSettings = componentOverridesSettings
	}

	// Extract env overrides.
	if i, ok := componentOverrides[cfg.EnvSectionName]; ok {
		componentOverridesEnv, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.overrides.env' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesEnv, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverridesEnv = componentOverridesEnv
	}

	// Extract auth overrides.
	if i, ok := componentOverrides[cfg.AuthSectionName]; ok {
		componentOverridesAuth, ok := i.(map[string]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.overrides.auth' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesAuth, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverridesAuth = componentOverridesAuth
	}

	// Extract command overrides.
	if i, ok := componentOverrides[cfg.CommandSectionName]; ok {
		componentOverridesCommand, ok := i.(string)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.overrides.command' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesCommand, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverridesCommand = componentOverridesCommand
	}

	// Terraform-specific: extract providers overrides.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := componentOverrides[cfg.ProvidersSectionName]; ok {
			componentOverridesProviders, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.overrides.providers' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesProviders, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesProviders = componentOverridesProviders
		}
	}

	// Terraform-specific: extract hooks overrides.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := componentOverrides[cfg.HooksSectionName]; ok {
			componentOverridesHooks, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.overrides.hooks' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesHooks, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesHooks = componentOverridesHooks
		}
	}

	return nil
}
