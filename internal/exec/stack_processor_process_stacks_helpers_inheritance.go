package exec

import (
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// processComponentInheritance processes component inheritance chains.
func processComponentInheritance(opts *ComponentProcessorOptions, result *ComponentProcessorResult) error {
	defer perf.Track(opts.AtmosConfig, "exec.processComponentInheritance")()

	// Initialize base component data with capacity hints.
	result.BaseComponentVars = make(map[string]any, componentSmallMapCapacity)
	result.BaseComponentSettings = make(map[string]any, componentSmallMapCapacity)
	result.BaseComponentEnv = make(map[string]any, componentSmallMapCapacity)
	result.BaseComponentAuth = make(map[string]any, componentSmallMapCapacity)
	result.BaseComponentMetadata = make(map[string]any, componentSmallMapCapacity)
	if opts.ComponentType == cfg.TerraformComponentType {
		result.BaseComponentProviders = make(map[string]any, componentSmallMapCapacity)
		result.BaseComponentRequiredProviders = make(map[string]any, componentSmallMapCapacity)
		result.BaseComponentHooks = make(map[string]any, componentSmallMapCapacity)
		result.BaseComponentBackendSection = make(map[string]any, componentSmallMapCapacity)
		result.BaseComponentRemoteStateBackendSection = make(map[string]any, componentSmallMapCapacity)
	}

	var baseComponentConfig schema.BaseComponentConfig
	var componentInheritanceChain []string

	// Process inheritance using the top-level `component` attribute.
	if err := processTopLevelComponentInheritance(opts, result, &baseComponentConfig, &componentInheritanceChain); err != nil {
		return err
	}

	// Process multiple inheritance using `metadata.inherits`.
	if err := processMetadataInheritance(opts, result, &baseComponentConfig, &componentInheritanceChain); err != nil {
		return err
	}

	result.BaseComponents = u.UniqueStrings(result.BaseComponents)
	sort.Strings(result.BaseComponents)
	result.ComponentInheritanceChain = componentInheritanceChain

	return nil
}

// processTopLevelComponentInheritance processes inheritance using the top-level `component` attribute.
func processTopLevelComponentInheritance(opts *ComponentProcessorOptions, result *ComponentProcessorResult, baseComponentConfig *schema.BaseComponentConfig, componentInheritanceChain *[]string) error {
	defer perf.Track(opts.AtmosConfig, "exec.processTopLevelComponentInheritance")()

	baseComponent, baseComponentExist := opts.ComponentMap[cfg.ComponentSectionName]
	if !baseComponentExist {
		return nil
	}

	baseComponentName, ok := baseComponent.(string)
	if !ok {
		return fmt.Errorf("%w: 'components.%s.%s.component' in the file '%s'", errUtils.ErrInvalidComponentAttribute, opts.ComponentType, opts.Component, opts.StackName)
	}

	// Process the base components recursively to find componentInheritanceChain.
	err := ProcessBaseComponentConfig(
		opts.AtmosConfig,
		baseComponentConfig,
		opts.AllComponentsMap,
		opts.Component,
		opts.Stack,
		baseComponentName,
		opts.ComponentsBasePath,
		opts.CheckBaseComponentExists,
		&result.BaseComponents,
	)
	if err != nil {
		return err
	}

	applyBaseComponentConfig(opts, result, baseComponentConfig, componentInheritanceChain)
	return nil
}

// processMetadataInheritance processes multiple inheritance using metadata.component and metadata.inherits.
func processMetadataInheritance(opts *ComponentProcessorOptions, result *ComponentProcessorResult, baseComponentConfig *schema.BaseComponentConfig, componentInheritanceChain *[]string) error {
	defer perf.Track(opts.AtmosConfig, "exec.processMetadataInheritance")()

	// Track whether `metadata.component` was explicitly set.
	// `metadata.component` is a pointer to the physical terraform component directory.
	// It is NOT inherited from base components - the `metadata` section is per-component.
	metadataComponentExplicitlySet := false
	explicitBaseComponentName := ""

	// Save the BaseComponentName from top-level component inheritance (if any).
	// Processing `metadata.inherits` calls applyBaseComponentConfig() which inadvertently overwrites.
	// BaseComponentName. We save it here so we can restore it after processing, distinguishing:
	// 1. BaseComponentName set by the top-level `component` attribute (preserve this value).
	// 2. BaseComponentName overwritten during configuration inheritance (restore saved value or default).
	baseComponentNameFromTopLevel := result.BaseComponentName

	// Check `metadata.component`.
	if baseComponentFromMetadata, baseComponentFromMetadataExist := result.ComponentMetadata[cfg.ComponentSectionName]; baseComponentFromMetadataExist {
		baseComponentName, ok := baseComponentFromMetadata.(string)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.metadata.component' in the file '%s'", errUtils.ErrInvalidComponentMetadataComponent, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.BaseComponentName = baseComponentName
		explicitBaseComponentName = baseComponentName
		metadataComponentExplicitlySet = true
	}

	// Process the `metadata.inherits` list (if it exists).
	// `metadata.inherits` specifies which Atmos components to inherit configuration from (vars, settings, env, etc.).
	inheritValue, inheritsKeyExists := result.ComponentMetadata[cfg.InheritsSectionName]
	if inheritsKeyExists {
		inheritList, ok := inheritValue.([]any)
		if !ok {
			return fmt.Errorf("%w: 'components.%s.%s.metadata.inherits' in the file '%s'", errUtils.ErrInvalidComponentMetadataInherits, opts.ComponentType, opts.Component, opts.StackName)
		}

		for _, v := range inheritList {
			if err := processInheritedComponent(opts, result, baseComponentConfig, componentInheritanceChain, v); err != nil {
				return err
			}
		}

		// Restore the explicitly set `metadata.component` after processing inherits.
		if metadataComponentExplicitlySet {
			result.BaseComponentName = explicitBaseComponentName
		}
	}

	// If `metadata.component` was not explicitly set, determine the default.
	// This logic is completely independent of metadata.inherits - metadata.inherits only affects
	// configuration inheritance (vars, settings, backend, etc.) and has nothing to do with component paths.
	if !metadataComponentExplicitlySet {
		if baseComponentNameFromTopLevel != "" {
			// Top-level component: attribute was set, restore it.
			// This handles cases like: `component: "component-1"`.
			result.BaseComponentName = baseComponentNameFromTopLevel
		} else {
			// No top-level `component` attribute, and no `metadata.component` - default to Atmos component name.
			result.BaseComponentName = opts.Component
		}
	}

	return nil
}

// processInheritedComponent processes a single inherited component from metadata.inherits list.
func processInheritedComponent(opts *ComponentProcessorOptions, result *ComponentProcessorResult, baseComponentConfig *schema.BaseComponentConfig, componentInheritanceChain *[]string, inheritValue any) error {
	defer perf.Track(opts.AtmosConfig, "exec.processInheritedComponent")()

	baseComponentFromInheritList, ok := inheritValue.(string)
	if !ok {
		return fmt.Errorf("%w: 'components.%s.%s.metadata.inherits' in the file '%s'", errUtils.ErrInvalidComponentMetadataInherits, opts.ComponentType, opts.Component, opts.StackName)
	}

	if _, ok := opts.AllComponentsMap[baseComponentFromInheritList]; !ok {
		if opts.CheckBaseComponentExists {
			return fmt.Errorf("%w: the component '%s' in the stack manifest '%s' inherits from '%s' (using 'metadata.inherits'), but '%s' is not defined in any of the config files for the stack '%s'",
				errUtils.ErrComponentNotDefined,
				opts.Component,
				opts.StackName,
				baseComponentFromInheritList,
				baseComponentFromInheritList,
				opts.StackName,
			)
		}
	}

	// Process the baseComponentFromInheritList components recursively.
	err := ProcessBaseComponentConfig(
		opts.AtmosConfig,
		baseComponentConfig,
		opts.AllComponentsMap,
		opts.Component,
		opts.Stack,
		baseComponentFromInheritList,
		opts.ComponentsBasePath,
		opts.CheckBaseComponentExists,
		&result.BaseComponents,
	)
	if err != nil {
		return err
	}

	applyBaseComponentConfig(opts, result, baseComponentConfig, componentInheritanceChain)
	return nil
}

// applyBaseComponentConfig applies the base component configuration to the result.
func applyBaseComponentConfig(opts *ComponentProcessorOptions, result *ComponentProcessorResult, baseComponentConfig *schema.BaseComponentConfig, componentInheritanceChain *[]string) {
	result.BaseComponentVars = baseComponentConfig.BaseComponentVars
	result.BaseComponentSettings = baseComponentConfig.BaseComponentSettings
	result.BaseComponentEnv = baseComponentConfig.BaseComponentEnv
	result.BaseComponentAuth = baseComponentConfig.BaseComponentAuth
	result.BaseComponentMetadata = baseComponentConfig.BaseComponentMetadata
	result.BaseComponentName = baseComponentConfig.FinalBaseComponentName
	result.BaseComponentCommand = baseComponentConfig.BaseComponentCommand
	*componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain

	// Terraform-specific: extract base component providers, hooks, backend, and source.
	if opts.ComponentType == cfg.TerraformComponentType {
		result.BaseComponentProviders = baseComponentConfig.BaseComponentProviders
		result.BaseComponentRequiredProviders = baseComponentConfig.BaseComponentRequiredProviders
		result.BaseComponentRequiredVersion = baseComponentConfig.BaseComponentRequiredVersion
		result.BaseComponentHooks = baseComponentConfig.BaseComponentHooks
		result.BaseComponentBackendType = baseComponentConfig.BaseComponentBackendType
		result.BaseComponentBackendSection = baseComponentConfig.BaseComponentBackendSection
		result.BaseComponentRemoteStateBackendType = baseComponentConfig.BaseComponentRemoteStateBackendType
		result.BaseComponentRemoteStateBackendSection = baseComponentConfig.BaseComponentRemoteStateBackendSection
		result.BaseComponentSourceSection = baseComponentConfig.BaseComponentSourceSection
	}
}
