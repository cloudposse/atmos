package exec

import (
	"fmt"

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

	// Merge vars using deferred merge to handle YAML functions.
	finalComponentVars, varsCtx, err := m.MergeWithDeferred(
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

	// Apply deferred merges for vars (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(varsCtx, finalComponentVars, atmosConfig, nil); err != nil {
		return nil, err
	}

	// Merge settings using deferred merge to handle YAML functions.
	finalComponentSettings, settingsCtx, err := m.MergeWithDeferred(
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

	// Apply deferred merges for settings (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(settingsCtx, finalComponentSettings, atmosConfig, nil); err != nil {
		return nil, err
	}

	// Merge env using deferred merge to handle YAML functions.
	finalComponentEnv, envCtx, err := m.MergeWithDeferred(
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

	// Apply deferred merges for env (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(envCtx, finalComponentEnv, atmosConfig, nil); err != nil {
		return nil, err
	}

	// Merge auth using deferred merge to handle YAML functions.
	finalComponentAuth, authCtx, err := m.MergeWithDeferred(
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

	// Apply deferred merges for auth (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(authCtx, finalComponentAuth, atmosConfig, nil); err != nil {
		return nil, err
	}

	// Terraform-specific: merge providers using deferred merge.
	var finalComponentProviders map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		var providersCtx *m.DeferredMergeContext
		finalComponentProviders, providersCtx, err = m.MergeWithDeferred(
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

		// Apply deferred merges for providers (without YAML processing - already done earlier).
		if err := m.ApplyDeferredMerges(providersCtx, finalComponentProviders, atmosConfig, nil); err != nil {
			return nil, err
		}
	}

	// Terraform-specific: merge hooks using deferred merge.
	var finalComponentHooks map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		var hooksCtx *m.DeferredMergeContext
		finalComponentHooks, hooksCtx, err = m.MergeWithDeferred(
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

		// Apply deferred merges for hooks (without YAML processing - already done earlier).
		if err := m.ApplyDeferredMerges(hooksCtx, finalComponentHooks, atmosConfig, nil); err != nil {
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

	// Merge metadata when inheritance is enabled.
	// Base component metadata is merged with component metadata.
	// Excluded from inheritance: 'inherits' and 'type' (already excluded during collection).
	finalComponentMetadata := result.ComponentMetadata
	if atmosConfig.Stacks.Inherit.IsMetadataInheritanceEnabled() && len(result.BaseComponentMetadata) > 0 {
		// Create a copy of base metadata excluding 'inherits' and 'type' (already excluded during collection).
		// Then merge with component metadata (component metadata wins on conflicts).
		finalComponentMetadata, err = m.Merge(
			atmosConfig,
			[]map[string]any{
				result.BaseComponentMetadata,
				result.ComponentMetadata,
			})
		if err != nil {
			return nil, err
		}
	}

	// Build final component map.
	comp := map[string]any{
		cfg.VarsSectionName:        finalComponentVars,
		cfg.SettingsSectionName:    finalSettings,
		cfg.EnvSectionName:         finalComponentEnv,
		cfg.AuthSectionName:        finalComponentAuth,
		cfg.CommandSectionName:     finalComponentCommand,
		cfg.InheritanceSectionName: result.ComponentInheritanceChain,
		cfg.MetadataSectionName:    finalComponentMetadata,
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
				componentMetadata:           finalComponentMetadata,
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
		mergedAuth, err := processAuthConfig(atmosConfig, opts.AtmosGlobalAuthMap, finalComponentAuth)
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

	// Process source and provision configuration for terraform, helmfile, and packer components.
	if opts.ComponentType == cfg.TerraformComponentType ||
		opts.ComponentType == cfg.HelmfileComponentType ||
		opts.ComponentType == cfg.PackerComponentType {
		finalComponentSource, err := m.Merge(
			atmosConfig,
			[]map[string]any{
				opts.GlobalSourceSection,
				result.BaseComponentSourceSection,
				result.ComponentSourceSection,
			})
		if err != nil {
			return nil, err
		}
		comp[cfg.SourceSectionName] = finalComponentSource
		comp[cfg.ProvisionSectionName] = result.ComponentProvision
	}

	// Add base component name if present.
	if result.BaseComponentName != "" {
		comp[cfg.ComponentSectionName] = result.BaseComponentName
	}

	return comp, nil
}

// processAuthConfig merges global and component-level auth configurations.
func processAuthConfig(atmosConfig *schema.AtmosConfiguration, globalAuthConfig map[string]any, authConfig map[string]any) (map[string]any, error) {
	// Use the pre-converted global auth config to avoid race conditions.
	// The globalAuthConfig parameter is pre-converted from atmosConfig.Auth before parallel processing starts.
	mergedAuthConfig, mergeCtx, err := m.MergeWithDeferred(
		atmosConfig,
		[]map[string]any{
			globalAuthConfig,
			authConfig,
		})
	if err != nil {
		return nil, fmt.Errorf("%w: merge auth config: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	// Apply deferred merges (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(mergeCtx, mergedAuthConfig, atmosConfig, nil); err != nil {
		return nil, fmt.Errorf("%w: apply deferred merges for auth config: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	return mergedAuthConfig, nil
}
