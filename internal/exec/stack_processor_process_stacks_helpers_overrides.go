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

	// Initialize overrides with small capacity hints (overrides are typically sparse).
	result.ComponentOverridesVars = make(map[string]any, componentOverridesCapacity)
	result.ComponentOverridesSettings = make(map[string]any, componentOverridesCapacity)
	result.ComponentOverridesEnv = make(map[string]any, componentOverridesCapacity)
	result.ComponentOverridesAuth = make(map[string]any, componentOverridesCapacity)
	if opts.ComponentType == cfg.TerraformComponentType {
		result.ComponentOverridesProviders = make(map[string]any, componentOverridesCapacity)
		result.ComponentOverridesRequiredProviders = make(map[string]any, componentOverridesCapacity)
		result.ComponentOverridesHooks = make(map[string]any, componentOverridesCapacity)
	}

	i, ok := opts.ComponentMap[cfg.OverridesSectionName]
	if !ok {
		result.ComponentOverrides = make(map[string]any, componentOverridesCapacity)
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

	// Terraform-specific: extract required_providers overrides (DEV-3124).
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := componentOverrides[cfg.RequiredProvidersSectionName]; ok {
			componentOverridesRequiredProviders, ok := i.(map[string]any)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.overrides.required_providers' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesRequiredProviders, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesRequiredProviders = componentOverridesRequiredProviders
		}
	}

	// Terraform-specific: extract required_version overrides (DEV-3124).
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := componentOverrides[cfg.RequiredVersionSectionName]; ok {
			componentOverridesRequiredVersion, ok := i.(string)
			if !ok {
				return fmt.Errorf("%w: 'components.%s.%s.overrides.required_version' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesRequiredVersion, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesRequiredVersion = componentOverridesRequiredVersion
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
