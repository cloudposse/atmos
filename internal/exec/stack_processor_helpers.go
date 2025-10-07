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

// ComponentProcessorOptions contains configuration for processing a component.
type ComponentProcessorOptions struct {
	ComponentType            string
	Component                string
	Stack                    string
	StackName                string
	ComponentMap             map[string]any
	AllComponentsMap         map[string]any
	ComponentsBasePath       string
	CheckBaseComponentExists bool

	// Global configurations.
	GlobalVars     map[string]any
	GlobalSettings map[string]any
	GlobalEnv      map[string]any
	GlobalCommand  string

	// Terraform-specific options.
	TerraformProviders              map[string]any
	GlobalAndTerraformHooks         map[string]any
	GlobalBackendType               string
	GlobalBackendSection            map[string]any
	GlobalRemoteStateBackendType    string
	GlobalRemoteStateBackendSection map[string]any

	// Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
}

// ComponentProcessorResult contains the processed component data.
type ComponentProcessorResult struct {
	ComponentVars              map[string]any
	ComponentSettings          map[string]any
	ComponentEnv               map[string]any
	ComponentMetadata          map[string]any
	ComponentCommand           string
	ComponentOverrides         map[string]any
	ComponentOverridesVars     map[string]any
	ComponentOverridesSettings map[string]any
	ComponentOverridesEnv      map[string]any
	ComponentOverridesCommand  string
	BaseComponentName          string
	BaseComponentVars          map[string]any
	BaseComponentSettings      map[string]any
	BaseComponentEnv           map[string]any
	BaseComponentCommand       string
	ComponentInheritanceChain  []string
	BaseComponents             []string

	// Terraform-specific fields.
	ComponentProviders                     map[string]any
	ComponentHooks                         map[string]any
	ComponentAuth                          map[string]any
	ComponentBackendType                   string
	ComponentBackendSection                map[string]any
	ComponentRemoteStateBackendType        string
	ComponentRemoteStateBackendSection     map[string]any
	ComponentOverridesProviders            map[string]any
	ComponentOverridesHooks                map[string]any
	BaseComponentProviders                 map[string]any
	BaseComponentHooks                     map[string]any
	BaseComponentBackendType               string
	BaseComponentBackendSection            map[string]any
	BaseComponentRemoteStateBackendType    string
	BaseComponentRemoteStateBackendSection map[string]any
}

// processComponent processes a component extracting common configuration sections.
func processComponent(opts ComponentProcessorOptions) (*ComponentProcessorResult, error) {
	defer perf.Track(opts.AtmosConfig, "exec.processComponent")()

	result := &ComponentProcessorResult{
		ComponentVars:     make(map[string]any),
		ComponentSettings: make(map[string]any),
		ComponentEnv:      make(map[string]any),
		ComponentMetadata: make(map[string]any),
		BaseComponents:    []string{},
	}

	// Extract vars section.
	if i, ok := opts.ComponentMap[cfg.VarsSectionName]; ok {
		componentVars, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.vars' in the file '%s'", errUtils.ErrInvalidComponentVars, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentVars = componentVars
	}

	// Extract settings section.
	if i, ok := opts.ComponentMap[cfg.SettingsSectionName]; ok {
		componentSettings, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.settings' in the file '%s'", errUtils.ErrInvalidComponentSettings, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentSettings = componentSettings

		// Terraform-specific: validate spacelift settings.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentSettings["spacelift"]; ok {
				_, ok = i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w: 'components.%s.%s.settings.spacelift' in the file '%s'", errUtils.ErrInvalidSpaceLiftSettings, opts.ComponentType, opts.Component, opts.StackName)
				}
			}
		}
	}

	// Extract env section.
	if i, ok := opts.ComponentMap[cfg.EnvSectionName]; ok {
		componentEnv, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.env' in the file '%s'", errUtils.ErrInvalidComponentEnv, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentEnv = componentEnv
	}

	// Terraform-specific: extract providers section.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.ProvidersSectionName]; ok {
			componentProviders, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.providers' in the file '%s'", errUtils.ErrInvalidComponentProviders, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentProviders = componentProviders
		} else {
			result.ComponentProviders = make(map[string]any)
		}
	}

	// Terraform-specific: extract hooks section.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.HooksSectionName]; ok {
			componentHooks, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.hooks' in the file '%s'", errUtils.ErrInvalidComponentHooks, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentHooks = componentHooks
		} else {
			result.ComponentHooks = make(map[string]any)
		}
	}

	// Extract auth section.
	if i, ok := opts.ComponentMap[cfg.AuthSectionName]; ok {
		componentAuth, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.auth' in the file '%s'", errUtils.ErrInvalidComponentAuth, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentAuth = componentAuth
	} else {
		result.ComponentAuth = make(map[string]any)
	}

	// Extract metadata section.
	if i, ok := opts.ComponentMap[cfg.MetadataSectionName]; ok {
		componentMetadata, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.metadata' in the file '%s'", errUtils.ErrInvalidComponentMetadata, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentMetadata = componentMetadata
	}

	// Terraform-specific: extract backend configuration.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.BackendTypeSectionName]; ok {
			componentBackendType, ok := i.(string)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.backend_type' in the file '%s'", errUtils.ErrInvalidComponentBackendType, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentBackendType = componentBackendType
		}

		if i, ok := opts.ComponentMap[cfg.BackendSectionName]; ok {
			componentBackendSection, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.backend' in the file '%s'", errUtils.ErrInvalidComponentBackend, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentBackendSection = componentBackendSection
		} else {
			result.ComponentBackendSection = make(map[string]any)
		}
	}

	// Terraform-specific: extract remote state backend configuration.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.RemoteStateBackendTypeSectionName]; ok {
			componentRemoteStateBackendType, ok := i.(string)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.remote_state_backend_type' in the file '%s'", errUtils.ErrInvalidComponentRemoteStateBackendType, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentRemoteStateBackendType = componentRemoteStateBackendType
		}

		if i, ok := opts.ComponentMap[cfg.RemoteStateBackendSectionName]; ok {
			componentRemoteStateBackendSection, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.remote_state_backend' in the file '%s'", errUtils.ErrInvalidComponentRemoteStateBackend, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentRemoteStateBackendSection = componentRemoteStateBackendSection
		} else {
			result.ComponentRemoteStateBackendSection = make(map[string]any)
		}
	}

	// Extract the executable command.
	if i, ok := opts.ComponentMap[cfg.CommandSectionName]; ok {
		componentCommand, ok := i.(string)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.command' in the file '%s'", errUtils.ErrInvalidComponentCommand, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentCommand = componentCommand
	}

	// Process overrides.
	result.ComponentOverridesVars = make(map[string]any)
	result.ComponentOverridesSettings = make(map[string]any)
	result.ComponentOverridesEnv = make(map[string]any)
	if opts.ComponentType == cfg.TerraformComponentType {
		result.ComponentOverridesProviders = make(map[string]any)
		result.ComponentOverridesHooks = make(map[string]any)
	}

	if i, ok := opts.ComponentMap[cfg.OverridesSectionName]; ok {
		componentOverrides, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.overrides' in the manifest '%s'", errUtils.ErrInvalidComponentOverrides, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverrides = componentOverrides

		if i, ok := componentOverrides[cfg.VarsSectionName]; ok {
			componentOverridesVars, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.overrides.vars' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesVars, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesVars = componentOverridesVars
		}

		if i, ok := componentOverrides[cfg.SettingsSectionName]; ok {
			componentOverridesSettings, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.overrides.settings' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesSettings, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesSettings = componentOverridesSettings
		}

		if i, ok := componentOverrides[cfg.EnvSectionName]; ok {
			componentOverridesEnv, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.overrides.env' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesEnv, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesEnv = componentOverridesEnv
		}

		if i, ok := componentOverrides[cfg.CommandSectionName]; ok {
			componentOverridesCommand, ok := i.(string)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.overrides.command' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesCommand, opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesCommand = componentOverridesCommand
		}

		// Terraform-specific: extract providers overrides.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentOverrides[cfg.ProvidersSectionName]; ok {
				componentOverridesProviders, ok := i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w: 'components.%s.%s.overrides.providers' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesProviders, opts.ComponentType, opts.Component, opts.StackName)
				}
				result.ComponentOverridesProviders = componentOverridesProviders
			}
		}

		// Terraform-specific: extract hooks overrides.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentOverrides[cfg.HooksSectionName]; ok {
				componentOverridesHooks, ok := i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%w: 'components.%s.%s.overrides.hooks' in the manifest '%s'", errUtils.ErrInvalidComponentOverridesHooks, opts.ComponentType, opts.Component, opts.StackName)
				}
				result.ComponentOverridesHooks = componentOverridesHooks
			}
		}
	} else {
		result.ComponentOverrides = make(map[string]any)
	}

	// Initialize base component data.
	result.BaseComponentVars = make(map[string]any)
	result.BaseComponentSettings = make(map[string]any)
	result.BaseComponentEnv = make(map[string]any)
	if opts.ComponentType == cfg.TerraformComponentType {
		result.BaseComponentProviders = make(map[string]any)
		result.BaseComponentHooks = make(map[string]any)
		result.BaseComponentBackendSection = make(map[string]any)
		result.BaseComponentRemoteStateBackendSection = make(map[string]any)
	}

	var baseComponentConfig schema.BaseComponentConfig
	var componentInheritanceChain []string

	// Process inheritance using the top-level `component` attribute.
	if baseComponent, baseComponentExist := opts.ComponentMap[cfg.ComponentSectionName]; baseComponentExist {
		baseComponentName, ok := baseComponent.(string)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.component' in the file '%s'", errUtils.ErrInvalidComponentAttribute, opts.ComponentType, opts.Component, opts.StackName)
		}

		// Process the base components recursively to find componentInheritanceChain.
		err := ProcessBaseComponentConfig(
			opts.AtmosConfig,
			&baseComponentConfig,
			opts.AllComponentsMap,
			opts.Component,
			opts.Stack,
			baseComponentName,
			opts.ComponentsBasePath,
			opts.CheckBaseComponentExists,
			&result.BaseComponents,
		)
		if err != nil {
			return nil, err
		}

		result.BaseComponentVars = baseComponentConfig.BaseComponentVars
		result.BaseComponentSettings = baseComponentConfig.BaseComponentSettings
		result.BaseComponentEnv = baseComponentConfig.BaseComponentEnv
		result.BaseComponentName = baseComponentConfig.FinalBaseComponentName
		result.BaseComponentCommand = baseComponentConfig.BaseComponentCommand
		componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain

		// Terraform-specific: extract base component providers, hooks, and backend.
		if opts.ComponentType == cfg.TerraformComponentType {
			result.BaseComponentProviders = baseComponentConfig.BaseComponentProviders
			result.BaseComponentHooks = baseComponentConfig.BaseComponentHooks
			result.BaseComponentBackendType = baseComponentConfig.BaseComponentBackendType
			result.BaseComponentBackendSection = baseComponentConfig.BaseComponentBackendSection
			result.BaseComponentRemoteStateBackendType = baseComponentConfig.BaseComponentRemoteStateBackendType
			result.BaseComponentRemoteStateBackendSection = baseComponentConfig.BaseComponentRemoteStateBackendSection
		}
	}

	// Multiple inheritance using metadata.component and metadata.inherits.
	if baseComponentFromMetadata, baseComponentFromMetadataExist := result.ComponentMetadata[cfg.ComponentSectionName]; baseComponentFromMetadataExist {
		baseComponentName, ok := baseComponentFromMetadata.(string)
		if !ok {
			return nil, fmt.Errorf("%w: 'components.%s.%s.metadata.component' in the file '%s'", errUtils.ErrInvalidComponentMetadataComponent, opts.ComponentType, opts.Component, opts.StackName)
		}
		result.BaseComponentName = baseComponentName
	}

	result.BaseComponents = append(result.BaseComponents, result.BaseComponentName)

	if inheritList, inheritListExist := result.ComponentMetadata[cfg.InheritsSectionName].([]any); inheritListExist {
		for _, v := range inheritList {
			baseComponentFromInheritList, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%w: 'components.%s.%s.metadata.inherits' in the file '%s'", errUtils.ErrInvalidComponentMetadataInherits, opts.ComponentType, opts.Component, opts.StackName)
			}

			if _, ok := opts.AllComponentsMap[baseComponentFromInheritList]; !ok {
				if opts.CheckBaseComponentExists {
					return nil, fmt.Errorf("%w: the component '%s' in the stack manifest '%s' inherits from '%s' (using 'metadata.inherits'), but '%s' is not defined in any of the config files for the stack '%s'",
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
				&baseComponentConfig,
				opts.AllComponentsMap,
				opts.Component,
				opts.Stack,
				baseComponentFromInheritList,
				opts.ComponentsBasePath,
				opts.CheckBaseComponentExists,
				&result.BaseComponents,
			)
			if err != nil {
				return nil, err
			}

			result.BaseComponentVars = baseComponentConfig.BaseComponentVars
			result.BaseComponentSettings = baseComponentConfig.BaseComponentSettings
			result.BaseComponentEnv = baseComponentConfig.BaseComponentEnv
			result.BaseComponentName = baseComponentConfig.FinalBaseComponentName
			result.BaseComponentCommand = baseComponentConfig.BaseComponentCommand
			componentInheritanceChain = baseComponentConfig.ComponentInheritanceChain

			// Terraform-specific: extract base component providers, hooks, and backend.
			if opts.ComponentType == cfg.TerraformComponentType {
				result.BaseComponentProviders = baseComponentConfig.BaseComponentProviders
				result.BaseComponentHooks = baseComponentConfig.BaseComponentHooks
				result.BaseComponentBackendType = baseComponentConfig.BaseComponentBackendType
				result.BaseComponentBackendSection = baseComponentConfig.BaseComponentBackendSection
				result.BaseComponentRemoteStateBackendType = baseComponentConfig.BaseComponentRemoteStateBackendType
				result.BaseComponentRemoteStateBackendSection = baseComponentConfig.BaseComponentRemoteStateBackendSection
			}
		}
	}

	result.BaseComponents = u.UniqueStrings(result.BaseComponents)
	sort.Strings(result.BaseComponents)
	result.ComponentInheritanceChain = componentInheritanceChain

	return result, nil
}
