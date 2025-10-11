package exec

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mergeComponentConfigurations merges component configurations (vars, settings, env, etc.).
//
//nolint:gocognit,nestif,revive,cyclop,funlen // Complex configuration merging logic with multiple component types.
func mergeComponentConfigurations(atmosConfig *schema.AtmosConfiguration, opts *ComponentProcessorOptions, result *ComponentProcessorResult) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.mergeComponentConfigurations")()

	// Merge vars.
	finalComponentVars, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			opts.GlobalVars,
			result.BaseComponentVars,
			result.ComponentVars,
			result.ComponentOverridesVars,
		})
	if err != nil {
		return nil, err
	}

	// Merge settings.
	finalComponentSettings, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			opts.GlobalSettings,
			result.BaseComponentSettings,
			result.ComponentSettings,
			result.ComponentOverridesSettings,
		})
	if err != nil {
		return nil, err
	}

	// Merge env.
	finalComponentEnv, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			opts.GlobalEnv,
			result.BaseComponentEnv,
			result.ComponentEnv,
			result.ComponentOverridesEnv,
		})
	if err != nil {
		return nil, err
	}

	// Merge auth.
	finalComponentAuth, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			opts.GlobalAuth,
			result.BaseComponentAuth,
			result.ComponentAuth,
			result.ComponentOverridesAuth,
		})
	if err != nil {
		return nil, err
	}

	// Terraform-specific: merge providers.
	var finalComponentProviders map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		finalComponentProviders, err = m.Merge(
			atmosConfig,
			[]map[string]any{
				opts.TerraformProviders,
				result.BaseComponentProviders,
				result.ComponentProviders,
				result.ComponentOverridesProviders,
			})
		if err != nil {
			return nil, err
		}
	}

	// Terraform-specific: merge hooks.
	var finalComponentHooks map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		finalComponentHooks, err = m.Merge(
			atmosConfig,
			[]map[string]any{
				opts.GlobalAndTerraformHooks,
				result.BaseComponentHooks,
				result.ComponentHooks,
				result.ComponentOverridesHooks,
			})
		if err != nil {
			return nil, err
		}
	}

	// Resolve the final executable command.
	// Check for the binary in the following order:
	// - `components.<type>.command` section in `atmos.yaml` CLI config file.
	// - global `<type>.command` section.
	// - base component(s) `command` section.
	// - component `command` section.
	// - `overrides.command` section.
	finalComponentCommand := opts.ComponentType
	if opts.ComponentType == cfg.TerraformComponentType && opts.AtmosConfig.Components.Terraform.Command != "" {
		finalComponentCommand = opts.AtmosConfig.Components.Terraform.Command
	}
	if opts.ComponentType == cfg.HelmfileComponentType && opts.AtmosConfig.Components.Helmfile.Command != "" {
		finalComponentCommand = opts.AtmosConfig.Components.Helmfile.Command
	}
	if opts.GlobalCommand != "" {
		finalComponentCommand = opts.GlobalCommand
	}
	if result.BaseComponentCommand != "" {
		finalComponentCommand = result.BaseComponentCommand
	}
	if result.ComponentCommand != "" {
		finalComponentCommand = result.ComponentCommand
	}
	if result.ComponentOverridesCommand != "" {
		finalComponentCommand = result.ComponentOverridesCommand
	}

	// Process settings integrations.
	finalSettings, err := processSettingsIntegrationsGithub(atmosConfig, finalComponentSettings)
	if err != nil {
		return nil, err
	}

	// Build final component map.
	comp := map[string]any{
		cfg.VarsSectionName:        finalComponentVars,
		cfg.SettingsSectionName:    finalSettings,
		cfg.EnvSectionName:         finalComponentEnv,
		cfg.AuthSectionName:        finalComponentAuth,
		cfg.CommandSectionName:     finalComponentCommand,
		cfg.InheritanceSectionName: result.ComponentInheritanceChain,
		cfg.MetadataSectionName:    result.ComponentMetadata,
		cfg.OverridesSectionName:   result.ComponentOverrides,
	}

	// Terraform-specific: process backends and add Terraform-specific fields.
	if opts.ComponentType == cfg.TerraformComponentType {
		// Process backend configuration.
		finalComponentBackendType, finalComponentBackend, err := processTerraformBackend(
			&terraformBackendConfig{
				atmosConfig:                 atmosConfig,
				component:                   opts.Component,
				baseComponentName:           result.BaseComponentName,
				globalBackendType:           opts.GlobalBackendType,
				globalBackendSection:        opts.GlobalBackendSection,
				baseComponentBackendType:    result.BaseComponentBackendType,
				baseComponentBackendSection: result.BaseComponentBackendSection,
				componentBackendType:        result.ComponentBackendType,
				componentBackendSection:     result.ComponentBackendSection,
			},
		)
		if err != nil {
			return nil, err
		}

		// Process remote state backend configuration.
		finalComponentRemoteStateBackendType, finalComponentRemoteStateBackend, err := processTerraformRemoteStateBackend(
			&remoteStateBackendConfig{
				atmosConfig:                            atmosConfig,
				component:                              opts.Component,
				finalComponentBackendType:              finalComponentBackendType,
				finalComponentBackendSection:           map[string]any{finalComponentBackendType: finalComponentBackend},
				globalRemoteStateBackendType:           opts.GlobalRemoteStateBackendType,
				globalRemoteStateBackendSection:        opts.GlobalRemoteStateBackendSection,
				baseComponentRemoteStateBackendType:    result.BaseComponentRemoteStateBackendType,
				baseComponentRemoteStateBackendSection: result.BaseComponentRemoteStateBackendSection,
				componentRemoteStateBackendType:        result.ComponentRemoteStateBackendType,
				componentRemoteStateBackendSection:     result.ComponentRemoteStateBackendSection,
			},
		)
		if err != nil {
			return nil, err
		}

		// Process auth configuration.
		mergedAuth, err := processAuthConfig(atmosConfig, finalComponentAuth)
		if err != nil {
			return nil, err
		}

		// Handle abstract components: remove spacelift workspace_enabled setting.
		componentIsAbstract := false
		if componentType, componentTypeAttributeExists := result.ComponentMetadata["type"].(string); componentTypeAttributeExists {
			if componentType == cfg.AbstractSectionName {
				componentIsAbstract = true
			}
		}
		if componentIsAbstract {
			if i, ok := finalSettings["spacelift"]; ok {
				spaceliftSettings, ok := i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w: 'components.%s.%s.settings.spacelift'", errUtils.ErrInvalidSpaceLiftSettings, opts.ComponentType, opts.Component)
				}
				delete(spaceliftSettings, "workspace_enabled")
			}
		}

		// Add Terraform-specific fields to component map.
		comp[cfg.ProvidersSectionName] = finalComponentProviders
		comp[cfg.HooksSectionName] = finalComponentHooks
		comp[cfg.BackendTypeSectionName] = finalComponentBackendType
		comp[cfg.BackendSectionName] = finalComponentBackend
		comp[cfg.RemoteStateBackendTypeSectionName] = finalComponentRemoteStateBackendType
		comp[cfg.RemoteStateBackendSectionName] = finalComponentRemoteStateBackend
		comp[cfg.AuthSectionName] = mergedAuth
	}

	// Add base component name if present.
	if result.BaseComponentName != "" {
		comp[cfg.ComponentSectionName] = result.BaseComponentName
	}

	return comp, nil
}

// processAuthConfig merges global and component-level auth configurations.
func processAuthConfig(atmosConfig *schema.AtmosConfiguration, authConfig map[string]any) (map[string]any, error) {
	// Convert the global auth config struct to map[string]any for merging.
	var globalAuthConfig map[string]any
	if err := mapstructure.Decode(atmosConfig.Auth, &globalAuthConfig); err != nil {
		return nil, fmt.Errorf("%w: failed to convert global auth config to map: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	mergedAuthConfig, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			globalAuthConfig,
			authConfig,
		})
	if err != nil {
		return nil, fmt.Errorf("%w: merge auth config: %v", errUtils.ErrInvalidAuthConfig, err)
	}

	return mergedAuthConfig, nil
}
