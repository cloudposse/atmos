package exec

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// tagSecretsScopes stamps the position-derived scope onto each secrets layer before merging: the
// stack-level (global) layer is `stack`-scoped, every component-level layer (base, component,
// overrides) is `instance`-scoped. It returns the layers in merge order (lowest→highest priority).
// A declaration whose explicit `scope` conflicts with its position is rejected as invalid secrets.
func tagSecretsScopes(global, base, component, overrides map[string]any) ([]map[string]any, error) {
	defer perf.Track(nil, "exec.tagSecretsScopes")()

	layers := []struct {
		section map[string]any
		scope   secrets.Scope
	}{
		{global, secrets.ScopeStack},
		{base, secrets.ScopeInstance},
		{component, secrets.ScopeInstance},
		{overrides, secrets.ScopeInstance},
	}
	out := make([]map[string]any, 0, len(layers))
	for _, l := range layers {
		tagged, err := secrets.TagScope(l.section, l.scope)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrInvalidComponentSecrets, err)
		}
		out = append(out, tagged)
	}
	return out, nil
}

// effectiveAtmosConfig returns an *AtmosConfiguration suitable for merging this
// component's sections. If any settings layer overrides list_merge_strategy, a
// shallow copy is returned with the new value; otherwise base is returned as-is
// (no allocation). Layers are applied in order — later entries win — matching
// the same precedence used when merging the settings section itself.
//
// Only Settings.ListMergeStrategy is written on the copy; all other fields
// remain shared with base. Do not mutate any other field on the returned copy
// without converting this to a deep copy first.
func effectiveAtmosConfig(base *schema.AtmosConfiguration, settingsLayers ...map[string]any) *schema.AtmosConfiguration {
	strategy := base.Settings.ListMergeStrategy
	for _, layer := range settingsLayers {
		if v, ok := layer["list_merge_strategy"].(string); ok && v != "" {
			strategy = v
		}
	}
	if strategy == base.Settings.ListMergeStrategy {
		return base
	}
	cfgCopy := *base
	cfgCopy.Settings.ListMergeStrategy = strategy
	return &cfgCopy
}

// mergeComponentConfigurations merges component configurations (vars, settings, env, etc.).
//
//nolint:gocognit,nestif,revive,cyclop,funlen // Complex configuration merging logic with multiple component types.
func mergeComponentConfigurations(atmosConfig *schema.AtmosConfiguration, opts *ComponentProcessorOptions, result *ComponentProcessorResult) (map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.mergeComponentConfigurations")()

	// Resolve the effective list_merge_strategy for this component before any merge.
	// Component-level settings (at any inheritance level) override the global atmos.yaml
	// setting, so individual components can opt into a different list merge behavior
	// without changing the global configuration. The layers are applied lowest-to-highest
	// priority: global settings → base component settings → component settings → overrides.
	// Using a local config copy avoids mutating the shared atmosConfig.
	mergeConfig := effectiveAtmosConfig(
		atmosConfig,
		opts.GlobalSettings,
		result.BaseComponentSettings,
		result.ComponentSettings,
		result.ComponentOverridesSettings,
	)

	// Merge vars using deferred merge to handle YAML functions.
	finalComponentVars, varsCtx, err := m.MergeWithDeferred(
		mergeConfig,
		[]map[string]any{
			opts.GlobalVars,
			result.BaseComponentVars,
			result.ComponentVars,
			result.ComponentOverridesVars,
		},
	)
	if err != nil {
		return nil, err
	}

	// Apply deferred merges for vars (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(varsCtx, finalComponentVars, mergeConfig, nil); err != nil {
		return nil, err
	}

	// Merge settings using deferred merge to handle YAML functions.
	finalComponentSettings, settingsCtx, err := m.MergeWithDeferred(
		mergeConfig,
		[]map[string]any{
			opts.GlobalSettings,
			result.BaseComponentSettings,
			result.ComponentSettings,
			result.ComponentOverridesSettings,
		},
	)
	if err != nil {
		return nil, err
	}

	// Apply deferred merges for settings (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(settingsCtx, finalComponentSettings, mergeConfig, nil); err != nil {
		return nil, err
	}

	// Merge env using deferred merge to handle YAML functions.
	finalComponentEnv, envCtx, err := m.MergeWithDeferred(
		mergeConfig,
		[]map[string]any{
			opts.GlobalEnv,
			result.BaseComponentEnv,
			result.ComponentEnv,
			result.ComponentOverridesEnv,
		},
	)
	if err != nil {
		return nil, err
	}

	// Apply deferred merges for env (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(envCtx, finalComponentEnv, mergeConfig, nil); err != nil {
		return nil, err
	}

	// Merge auth using deferred merge to handle YAML functions.
	finalComponentAuth, authCtx, err := m.MergeWithDeferred(
		mergeConfig,
		[]map[string]any{
			opts.GlobalAuth,
			result.BaseComponentAuth,
			result.ComponentAuth,
			result.ComponentOverridesAuth,
		},
	)
	if err != nil {
		return nil, err
	}

	// Apply deferred merges for auth (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(authCtx, finalComponentAuth, mergeConfig, nil); err != nil {
		return nil, err
	}

	// Terraform-specific: merge providers using deferred merge.
	var finalComponentProviders map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		var providersCtx *m.DeferredMergeContext
		finalComponentProviders, providersCtx, err = m.MergeWithDeferred(
			mergeConfig,
			[]map[string]any{
				opts.TerraformProviders,
				result.BaseComponentProviders,
				result.ComponentProviders,
				result.ComponentOverridesProviders,
			},
		)
		if err != nil {
			return nil, err
		}

		// Apply deferred merges for providers (without YAML processing - already done earlier).
		if err := m.ApplyDeferredMerges(providersCtx, finalComponentProviders, mergeConfig, nil); err != nil {
			return nil, err
		}
	}

	// Terraform-specific: merge required_providers using deferred merge (DEV-3124).
	var finalComponentRequiredProviders map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		var requiredProvidersCtx *m.DeferredMergeContext
		finalComponentRequiredProviders, requiredProvidersCtx, err = m.MergeWithDeferred(
			mergeConfig,
			[]map[string]any{
				opts.TerraformRequiredProviders,
				result.BaseComponentRequiredProviders,
				result.ComponentRequiredProviders,
				result.ComponentOverridesRequiredProviders,
			},
		)
		if err != nil {
			return nil, err
		}

		// Apply deferred merges for required_providers (without YAML processing - already done earlier).
		if err := m.ApplyDeferredMerges(requiredProvidersCtx, finalComponentRequiredProviders, mergeConfig, nil); err != nil {
			return nil, err
		}
	}

	// Terraform-specific: resolve required_version (DEV-3124).
	// Uses the same precedence as command: global -> base component -> component -> overrides.
	var finalComponentRequiredVersion string
	if opts.ComponentType == cfg.TerraformComponentType {
		if opts.TerraformRequiredVersion != "" {
			finalComponentRequiredVersion = opts.TerraformRequiredVersion
		}
		if result.BaseComponentRequiredVersion != "" {
			finalComponentRequiredVersion = result.BaseComponentRequiredVersion
		}
		if result.ComponentRequiredVersion != "" {
			finalComponentRequiredVersion = result.ComponentRequiredVersion
		}
		if result.ComponentOverridesRequiredVersion != "" {
			finalComponentRequiredVersion = result.ComponentOverridesRequiredVersion
		}
	}

	// Merge hooks using deferred merge for component types with lifecycle hooks.
	var finalComponentHooks map[string]any
	if supportsComponentHooks(opts.ComponentType) {
		var hooksCtx *m.DeferredMergeContext
		finalComponentHooks, hooksCtx, err = m.MergeWithDeferred(
			mergeConfig,
			[]map[string]any{
				opts.GlobalAndTerraformHooks,
				result.BaseComponentHooks,
				result.ComponentHooks,
				result.ComponentOverridesHooks,
			},
		)
		if err != nil {
			return nil, err
		}

		// Apply deferred merges for hooks (without YAML processing - already done earlier).
		if err := m.ApplyDeferredMerges(hooksCtx, finalComponentHooks, mergeConfig, nil); err != nil {
			return nil, err
		}
	}

	// Terraform-specific: merge test configuration.
	var finalComponentTest map[string]any
	if opts.ComponentType == cfg.TerraformComponentType {
		var testCtx *m.DeferredMergeContext
		finalComponentTest, testCtx, err = m.MergeWithDeferred(
			mergeConfig,
			[]map[string]any{
				result.BaseComponentTest,
				result.ComponentTest,
			},
		)
		if err != nil {
			return nil, err
		}

		if err := m.ApplyDeferredMerges(testCtx, finalComponentTest, mergeConfig, nil); err != nil {
			return nil, err
		}
	}

	// Merge secrets declarations (global stack → base → component → overrides). Available for
	// all component types; inherits through the stack hierarchy like other sections. The
	// stack-level (global) `secrets:` block lets providers/declarations be defined once per stack.
	//
	// Scope is derived from position and stamped onto each declaration BEFORE the merge: the
	// global (stack-level) layer is `stack`-scoped, every component-level layer is `instance`-scoped.
	// "Most-specific wins" then resolves overrides — a component re-declaring a stack secret pulls it
	// to instance scope — and enforces the one-way rule, with no merge-engine changes.
	var finalComponentSecrets map[string]any
	if len(opts.GlobalSecrets) > 0 || len(result.BaseComponentSecrets) > 0 || len(result.ComponentSecrets) > 0 || len(result.ComponentOverridesSecrets) > 0 {
		scopedSecrets, err := tagSecretsScopes(opts.GlobalSecrets, result.BaseComponentSecrets, result.ComponentSecrets, result.ComponentOverridesSecrets)
		if err != nil {
			return nil, err
		}
		finalComponentSecrets, err = m.Merge(mergeConfig, scopedSecrets)
		if err != nil {
			return nil, err
		}
	}

	// Merge generate section using deferred merge.
	// Merge order (lowest to highest priority):
	// 1. Global + component-type-level generate
	// 2. Base component generate (from metadata.inherits)
	// 3. Component generate (component-specific generate section)
	// 4. Component overrides generate (from overrides section)
	var finalComponentGenerate map[string]any
	if supportsGenerate(opts.ComponentType) {
		var generateCtx *m.DeferredMergeContext
		finalComponentGenerate, generateCtx, err = m.MergeWithDeferred(
			mergeConfig,
			[]map[string]any{
				opts.GlobalAndTerraformGenerate,
				result.BaseComponentGenerate,
				result.ComponentGenerate,
				result.ComponentOverridesGenerate,
			},
		)
		if err != nil {
			return nil, err
		}

		// Apply deferred merges for generate (without YAML processing - already done earlier).
		if err := m.ApplyDeferredMerges(generateCtx, finalComponentGenerate, mergeConfig, nil); err != nil {
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

	// Precedence (lowest to highest): stack-global Kubernetes defaults → base
	// component → component instance.
	finalComponentProvider := opts.GlobalKubernetesProvider
	if result.BaseComponentProvider != "" {
		finalComponentProvider = result.BaseComponentProvider
	}
	if result.ComponentProvider != "" {
		finalComponentProvider = result.ComponentProvider
	}

	finalComponentPaths, err := mergeComponentAnySection(
		mergeConfig,
		cfg.PathsSectionName,
		opts.GlobalKubernetesPaths,
		result.BaseComponentPaths,
		result.ComponentPaths,
	)
	if err != nil {
		return nil, err
	}

	finalComponentManifests, err := mergeComponentAnySection(
		mergeConfig,
		cfg.ManifestsSectionName,
		opts.GlobalKubernetesManifests,
		result.BaseComponentManifests,
		result.ComponentManifests,
	)
	if err != nil {
		return nil, err
	}

	var finalComponentRender map[string]any
	if opts.ComponentType == cfg.KubernetesComponentType {
		finalComponentRender, err = m.Merge(
			mergeConfig,
			[]map[string]any{
				opts.GlobalKubernetesRender,
				result.BaseComponentRender,
				result.ComponentRender,
			},
		)
		if err != nil {
			return nil, err
		}
	}

	// Process settings integrations.
	finalSettings, err := processSettingsIntegrationsGithub(mergeConfig, finalComponentSettings)
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
			mergeConfig,
			[]map[string]any{
				result.BaseComponentMetadata,
				result.ComponentMetadata,
			},
		)
		if err != nil {
			return nil, err
		}
	}

	// Merge dependencies (global + base component + component dependencies).
	// Priority (lowest to highest): global/component-type → base component → component instance.
	var finalComponentDependencies map[string]any
	if len(opts.GlobalDependencies) > 0 || len(result.BaseComponentDependencies) > 0 || len(result.ComponentDependencies) > 0 {
		finalComponentDependencies, err = m.Merge(
			mergeConfig,
			[]map[string]any{
				opts.GlobalDependencies,
				result.BaseComponentDependencies,
				result.ComponentDependencies,
			},
		)
		if err != nil {
			return nil, err
		}
	}

	// Merge locals (base component locals + component locals).
	// Component locals take precedence over base component locals.
	// Note: Locals are used for template processing, not passed to terraform/helmfile.
	var finalComponentLocals map[string]any
	if len(result.BaseComponentLocals) > 0 || len(result.ComponentLocals) > 0 {
		finalComponentLocals, err = m.Merge(
			mergeConfig,
			[]map[string]any{
				result.BaseComponentLocals,
				result.ComponentLocals,
			},
		)
		if err != nil {
			return nil, err
		}
	}

	// Merge retry config (base → component → overrides).
	// Deep-merge handles top-level scalars (max_attempts, backoff_strategy, ...).
	// The list-valued `conditions:` field follows the project's configured
	// `settings.list_merge_strategy` (default: replace), so by default a concrete
	// component's conditions list replaces the inherited one rather than extending it.
	// Users who want additive conditions can opt in by setting list_merge_strategy: append.
	var finalComponentRetry map[string]any
	if len(result.BaseComponentRetry) > 0 || len(result.ComponentRetry) > 0 || len(result.ComponentOverridesRetry) > 0 {
		finalComponentRetry, err = m.Merge(
			atmosConfig,
			[]map[string]any{
				result.BaseComponentRetry,
				result.ComponentRetry,
				result.ComponentOverridesRetry,
			},
		)
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

	// Add hooks for every component type that supports lifecycle hooks — kept
	// here in one place (rather than duplicated per type-specific block below)
	// so a new hooks-capable component type only needs to be added to
	// supportsComponentHooks.
	if supportsComponentHooks(opts.ComponentType) {
		comp[cfg.HooksSectionName] = finalComponentHooks
	}

	// Add dependencies if present.
	if len(finalComponentDependencies) > 0 {
		comp[cfg.DependenciesSectionName] = finalComponentDependencies
	}

	// Add locals if present (for template processing, not passed to terraform/helmfile).
	if len(finalComponentLocals) > 0 {
		comp[cfg.LocalsSectionName] = finalComponentLocals
	}

	// Add retry config if present.
	if len(finalComponentRetry) > 0 {
		comp[cfg.RetrySectionName] = finalComponentRetry
	}

	// Add secrets declarations if present (all component types).
	if len(finalComponentSecrets) > 0 {
		comp[cfg.SecretsSectionName] = finalComponentSecrets
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
		mergedAuth, err := processAuthConfig(mergeConfig, opts.AtmosGlobalAuthMap, finalComponentAuth)
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
		comp[cfg.RequiredProvidersSectionName] = finalComponentRequiredProviders
		comp[cfg.RequiredVersionSectionName] = finalComponentRequiredVersion
		if len(finalComponentTest) > 0 {
			comp[cfg.TestSectionName] = finalComponentTest
		}
		comp[cfg.GenerateSectionName] = finalComponentGenerate
		comp[cfg.BackendTypeSectionName] = finalComponentBackendType
		comp[cfg.BackendSectionName] = finalComponentBackend
		comp[cfg.RemoteStateBackendTypeSectionName] = finalComponentRemoteStateBackendType
		comp[cfg.RemoteStateBackendSectionName] = finalComponentRemoteStateBackend
		comp[cfg.AuthSectionName] = mergedAuth
	}

	if opts.ComponentType == cfg.KubernetesComponentType {
		if finalComponentProvider != "" {
			comp[cfg.ProviderSectionName] = finalComponentProvider
		}
		if finalComponentPaths != nil {
			comp[cfg.PathsSectionName] = finalComponentPaths
		}
		if finalComponentManifests != nil {
			comp[cfg.ManifestsSectionName] = finalComponentManifests
		}
		if len(finalComponentRender) > 0 {
			comp[cfg.RenderSectionName] = finalComponentRender
		}
		comp[cfg.GenerateSectionName] = finalComponentGenerate
	}

	if opts.ComponentType == cfg.HelmComponentType {
		finalComponentHelm, err := m.Merge(
			mergeConfig,
			[]map[string]any{
				result.BaseComponentHelm,
				result.ComponentHelm,
			},
		)
		if err != nil {
			return nil, err
		}
		for key, value := range finalComponentHelm {
			comp[key] = value
		}
		comp[cfg.GenerateSectionName] = finalComponentGenerate
	}

	// Merge the Helm CLI plugins list (helm and helmfile components).
	// Base-component plugins (e.g. from an abstract/catalog component) are merged
	// with the concrete component's plugins; the configured list_merge_strategy
	// (default: replace) governs how the lists combine.
	if supportsPlugins(opts.ComponentType) {
		finalComponentPlugins, err := mergeComponentAnySection(
			mergeConfig,
			cfg.PluginsSectionName,
			result.BaseComponentPlugins,
			result.ComponentPlugins,
		)
		if err != nil {
			return nil, err
		}
		if finalComponentPlugins != nil {
			comp[cfg.PluginsSectionName] = finalComponentPlugins
		}
	}

	// Process source and provision configuration.
	if supportsSourceProvision(opts.ComponentType) {
		finalComponentSource, err := m.Merge(
			mergeConfig,
			[]map[string]any{
				opts.GlobalSourceSection,
				result.BaseComponentSourceSection,
				result.ComponentSourceSection,
			},
		)
		if err != nil {
			return nil, err
		}
		comp[cfg.SourceSectionName] = finalComponentSource

		// Merge provision from global, base component, and component levels.
		// Priority (lowest to highest): global → base component → component.
		finalComponentProvision, err := m.Merge(
			mergeConfig,
			[]map[string]any{
				opts.GlobalProvisionSection,
				result.BaseComponentProvisionSection,
				result.ComponentProvision,
			},
		)
		if err != nil {
			return nil, err
		}
		comp[cfg.ProvisionSectionName] = finalComponentProvision
	}

	// Add base component name if present.
	if result.BaseComponentName != "" {
		comp[cfg.ComponentSectionName] = result.BaseComponentName
	}

	return comp, nil
}

func mergeComponentAnySection(atmosConfig *schema.AtmosConfiguration, key string, values ...any) (any, error) {
	sections := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		sections = append(sections, map[string]any{key: value})
	}
	if len(sections) == 0 {
		return nil, nil
	}
	merged, err := m.Merge(atmosConfig, sections)
	if err != nil {
		return nil, err
	}
	return merged[key], nil
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
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: merge auth config: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	// Apply deferred merges (without YAML processing - already done earlier).
	if err := m.ApplyDeferredMerges(mergeCtx, mergedAuthConfig, atmosConfig, nil); err != nil {
		return nil, fmt.Errorf("%w: apply deferred merges for auth config: %w", errUtils.ErrInvalidAuthConfig, err)
	}

	return mergedAuthConfig, nil
}
