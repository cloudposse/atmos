package exec

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ComponentProcessorOptions contains configuration for processing a component.
type ComponentProcessorOptions struct {
	ComponentType          string
	Component              string
	Stack                  string
	StackName              string
	ComponentMap           map[string]any
	AllComponentsMap       map[string]any
	ComponentsBasePath     string
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
	ComponentVars                map[string]any
	ComponentSettings            map[string]any
	ComponentEnv                 map[string]any
	ComponentMetadata            map[string]any
	ComponentCommand             string
	ComponentOverrides           map[string]any
	ComponentOverridesVars       map[string]any
	ComponentOverridesSettings   map[string]any
	ComponentOverridesEnv        map[string]any
	ComponentOverridesCommand    string
	BaseComponentName            string
	BaseComponentVars            map[string]any
	BaseComponentSettings        map[string]any
	BaseComponentEnv             map[string]any
	BaseComponentCommand         string
	ComponentInheritanceChain    []string
	BaseComponents               []string

	// Terraform-specific fields.
	ComponentProviders              map[string]any
	ComponentHooks                  map[string]any
	ComponentAuth                   map[string]any
	ComponentBackendType            string
	ComponentBackendSection         map[string]any
	ComponentRemoteStateBackendType string
	ComponentRemoteStateBackendSection map[string]any
	ComponentOverridesProviders     map[string]any
	ComponentOverridesHooks         map[string]any
	BaseComponentProviders          map[string]any
	BaseComponentHooks              map[string]any
	BaseComponentBackendType        string
	BaseComponentBackendSection     map[string]any
	BaseComponentRemoteStateBackendType string
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
			return nil, fmt.Errorf("invalid 'components.%s.%s.vars' section in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentVars = componentVars
	}

	// Extract settings section.
	if i, ok := opts.ComponentMap[cfg.SettingsSectionName]; ok {
		componentSettings, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'components.%s.%s.settings' section in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentSettings = componentSettings

		// Terraform-specific: validate spacelift settings.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentSettings["spacelift"]; ok {
				_, ok = i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.terraform.%s.settings.spacelift' section in the file '%s'", opts.Component, opts.StackName)
				}
			}
		}
	}

	// Extract env section.
	if i, ok := opts.ComponentMap[cfg.EnvSectionName]; ok {
		componentEnv, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'components.%s.%s.env' section in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentEnv = componentEnv
	}

	// Terraform-specific: extract providers section.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.ProvidersSectionName]; ok {
			componentProviders, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform.%s.providers' section in the file '%s'", opts.Component, opts.StackName)
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
				return nil, fmt.Errorf("invalid 'components.terraform.%s.hooks' section in the file '%s'", opts.Component, opts.StackName)
			}
			result.ComponentHooks = componentHooks
		} else {
			result.ComponentHooks = make(map[string]any)
		}
	}

	// Terraform-specific: extract auth section.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.AuthSectionName]; ok {
			componentAuth, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform.%s.auth' section in the file '%s'", opts.Component, opts.StackName)
			}
			result.ComponentAuth = componentAuth
		} else {
			result.ComponentAuth = make(map[string]any)
		}
	}

	// Extract metadata section.
	if i, ok := opts.ComponentMap[cfg.MetadataSectionName]; ok {
		componentMetadata, ok := i.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid 'components.%s.%s.metadata' section in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentMetadata = componentMetadata
	}

	// Terraform-specific: extract backend configuration.
	if opts.ComponentType == cfg.TerraformComponentType {
		if i, ok := opts.ComponentMap[cfg.BackendTypeSectionName]; ok {
			componentBackendType, ok := i.(string)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform.%s.backend_type' attribute in the file '%s'", opts.Component, opts.StackName)
			}
			result.ComponentBackendType = componentBackendType
		}

		if i, ok := opts.ComponentMap[cfg.BackendSectionName]; ok {
			componentBackendSection, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform.%s.backend' section in the file '%s'", opts.Component, opts.StackName)
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
				return nil, fmt.Errorf("invalid 'components.terraform.%s.remote_state_backend_type' attribute in the file '%s'", opts.Component, opts.StackName)
			}
			result.ComponentRemoteStateBackendType = componentRemoteStateBackendType
		}

		if i, ok := opts.ComponentMap[cfg.RemoteStateBackendSectionName]; ok {
			componentRemoteStateBackendSection, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.terraform.%s.remote_state_backend' section in the file '%s'", opts.Component, opts.StackName)
			}
			result.ComponentRemoteStateBackendSection = componentRemoteStateBackendSection
		} else {
			result.ComponentRemoteStateBackendSection = make(map[string]any)
		}
	}

	// Extract command.
	if i, ok := opts.ComponentMap[cfg.CommandSectionName]; ok {
		componentCommand, ok := i.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'components.%s.%s.command' attribute in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
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
			return nil, fmt.Errorf("invalid 'components.%s.%s.overrides' in the manifest '%s'", opts.ComponentType, opts.Component, opts.StackName)
		}
		result.ComponentOverrides = componentOverrides

		if i, ok := componentOverrides[cfg.VarsSectionName]; ok {
			componentOverridesVars, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.%s.%s.overrides.vars' in the manifest '%s'", opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesVars = componentOverridesVars
		}

		if i, ok := componentOverrides[cfg.SettingsSectionName]; ok {
			componentOverridesSettings, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.%s.%s.overrides.settings' in the manifest '%s'", opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesSettings = componentOverridesSettings
		}

		if i, ok := componentOverrides[cfg.EnvSectionName]; ok {
			componentOverridesEnv, ok := i.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.%s.%s.overrides.env' in the manifest '%s'", opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesEnv = componentOverridesEnv
		}

		if i, ok := componentOverrides[cfg.CommandSectionName]; ok {
			componentOverridesCommand, ok := i.(string)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.%s.%s.overrides.command' in the manifest '%s'", opts.ComponentType, opts.Component, opts.StackName)
			}
			result.ComponentOverridesCommand = componentOverridesCommand
		}

		// Terraform-specific: extract providers overrides.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentOverrides[cfg.ProvidersSectionName]; ok {
				componentOverridesProviders, ok := i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.providers' in the manifest '%s'", opts.Component, opts.StackName)
				}
				result.ComponentOverridesProviders = componentOverridesProviders
			}
		}

		// Terraform-specific: extract hooks overrides.
		if opts.ComponentType == cfg.TerraformComponentType {
			if i, ok := componentOverrides[cfg.HooksSectionName]; ok {
				componentOverridesHooks, ok := i.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid 'components.terraform.%s.overrides.hooks' in the manifest '%s'", opts.Component, opts.StackName)
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
			return nil, fmt.Errorf("invalid 'components.%s.%s.component' attribute in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
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
			return nil, fmt.Errorf("invalid 'components.%s.%s.metadata.component' attribute in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
		}
		result.BaseComponentName = baseComponentName
	}

	result.BaseComponents = append(result.BaseComponents, result.BaseComponentName)

	if inheritList, inheritListExist := result.ComponentMetadata[cfg.InheritsSectionName].([]any); inheritListExist {
		for _, v := range inheritList {
			baseComponentFromInheritList, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("invalid 'components.%s.%s.metadata.inherits' section in the file '%s'", opts.ComponentType, opts.Component, opts.StackName)
			}

			if _, ok := opts.AllComponentsMap[baseComponentFromInheritList]; !ok {
				if opts.CheckBaseComponentExists {
					errorMessage := fmt.Sprintf("The component '%[1]s' in the stack manifest '%[2]s' inherits from '%[3]s' "+
						"(using 'metadata.inherits'), but '%[3]s' is not defined in any of the config files for the stack '%[2]s'",
						opts.Component,
						opts.StackName,
						baseComponentFromInheritList,
					)
					return nil, errors.New(errorMessage)
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

// processTerraformBackend processes Terraform backend configuration including S3, GCS, and Azure backends.
func processTerraformBackend(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	baseComponentName string,
	globalBackendType string,
	globalBackendSection map[string]any,
	baseComponentBackendType string,
	baseComponentBackendSection map[string]any,
	componentBackendType string,
	componentBackendSection map[string]any,
) (string, map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.processTerraformBackend")()

	// Determine final backend type.
	finalComponentBackendType := globalBackendType
	if len(baseComponentBackendType) > 0 {
		finalComponentBackendType = baseComponentBackendType
	}
	if len(componentBackendType) > 0 {
		finalComponentBackendType = componentBackendType
	}

	// Merge backend sections.
	finalComponentBackendSection, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			globalBackendSection,
			baseComponentBackendSection,
			componentBackendSection,
		})
	if err != nil {
		return "", nil, err
	}

	// Extract backend configuration for the specific backend type.
	finalComponentBackend := map[string]any{}
	if i, ok := finalComponentBackendSection[finalComponentBackendType]; ok {
		finalComponentBackend, ok = i.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("invalid 'terraform.backend' section for the component '%s'", component)
		}
	}

	// AWS S3 backend: Set workspace_key_prefix if not present.
	if finalComponentBackendType == "s3" {
		if p, ok := finalComponentBackend["workspace_key_prefix"].(string); !ok || p == "" {
			workspaceKeyPrefix := component
			if baseComponentName != "" {
				workspaceKeyPrefix = baseComponentName
			}
			finalComponentBackend["workspace_key_prefix"] = strings.Replace(workspaceKeyPrefix, "/", "-", -1)
		}
	}

	// Google GCS backend: Set prefix if not present.
	if finalComponentBackendType == "gcs" {
		if p, ok := finalComponentBackend["prefix"].(string); !ok || p == "" {
			prefix := component
			if baseComponentName != "" {
				prefix = baseComponentName
			}
			finalComponentBackend["prefix"] = strings.Replace(prefix, "/", "-", -1)
		}
	}

	// Azure backend: Set key if not present.
	if finalComponentBackendType == "azurerm" {
		componentAzurerm, componentAzurermExists := componentBackendSection["azurerm"].(map[string]any)
		if !componentAzurermExists {
			componentAzurerm = map[string]any{}
		}
		if _, componentAzurermKeyExists := componentAzurerm["key"].(string); !componentAzurermKeyExists {
			azureKeyPrefixComponent := component
			var keyName []string
			if baseComponentName != "" {
				azureKeyPrefixComponent = baseComponentName
			}
			if globalAzurerm, globalAzurermExists := globalBackendSection["azurerm"].(map[string]any); globalAzurermExists {
				if _, globalAzurermKeyExists := globalAzurerm["key"].(string); globalAzurermKeyExists {
					keyName = append(keyName, globalAzurerm["key"].(string))
				}
			}
			componentKeyName := strings.ReplaceAll(azureKeyPrefixComponent, "/", "-")
			keyName = append(keyName, fmt.Sprintf("%s.terraform.tfstate", componentKeyName))
			finalComponentBackend["key"] = strings.Join(keyName, "/")
		}
	}

	return finalComponentBackendType, finalComponentBackend, nil
}

// processTerraformRemoteStateBackend processes Terraform remote state backend configuration.
func processTerraformRemoteStateBackend(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	finalComponentBackendType string,
	finalComponentBackendSection map[string]any,
	globalRemoteStateBackendType string,
	globalRemoteStateBackendSection map[string]any,
	baseComponentRemoteStateBackendType string,
	baseComponentRemoteStateBackendSection map[string]any,
	componentRemoteStateBackendType string,
	componentRemoteStateBackendSection map[string]any,
) (string, map[string]any, error) {
	defer perf.Track(atmosConfig, "exec.processTerraformRemoteStateBackend")()

	// Determine final remote state backend type.
	finalComponentRemoteStateBackendType := finalComponentBackendType
	if len(globalRemoteStateBackendType) > 0 {
		finalComponentRemoteStateBackendType = globalRemoteStateBackendType
	}
	if len(baseComponentRemoteStateBackendType) > 0 {
		finalComponentRemoteStateBackendType = baseComponentRemoteStateBackendType
	}
	if len(componentRemoteStateBackendType) > 0 {
		finalComponentRemoteStateBackendType = componentRemoteStateBackendType
	}

	// Merge remote state backend sections.
	finalComponentRemoteStateBackendSection, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			globalRemoteStateBackendSection,
			baseComponentRemoteStateBackendSection,
			componentRemoteStateBackendSection,
		})
	if err != nil {
		return "", nil, err
	}

	// Merge backend and remote_state_backend sections for DRY configuration.
	finalComponentRemoteStateBackendSectionMerged, err := m.Merge(
		atmosConfig,
		[]map[string]any{
			finalComponentBackendSection,
			finalComponentRemoteStateBackendSection,
		})
	if err != nil {
		return "", nil, err
	}

	// Extract remote state backend configuration for the specific backend type.
	finalComponentRemoteStateBackend := map[string]any{}
	if i, ok := finalComponentRemoteStateBackendSectionMerged[finalComponentRemoteStateBackendType]; ok {
		finalComponentRemoteStateBackend, ok = i.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("invalid 'terraform.remote_state_backend' section for the component '%s'", component)
		}
	}

	return finalComponentRemoteStateBackendType, finalComponentRemoteStateBackend, nil
}

// mergeComponentConfigurations merges component configurations (vars, settings, env, etc.).
func mergeComponentConfigurations(atmosConfig *schema.AtmosConfiguration, opts ComponentProcessorOptions, result *ComponentProcessorResult) (map[string]any, error) {
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

	// Resolve final command.
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
		cfg.CommandSectionName:     finalComponentCommand,
		cfg.InheritanceSectionName: result.ComponentInheritanceChain,
		cfg.MetadataSectionName:    result.ComponentMetadata,
		cfg.OverridesSectionName:   result.ComponentOverrides,
	}

	// Terraform-specific: process backends and add Terraform-specific fields.
	if opts.ComponentType == cfg.TerraformComponentType {
		// Process backend configuration.
		finalComponentBackendType, finalComponentBackend, err := processTerraformBackend(
			atmosConfig,
			opts.Component,
			result.BaseComponentName,
			opts.GlobalBackendType,
			opts.GlobalBackendSection,
			result.BaseComponentBackendType,
			result.BaseComponentBackendSection,
			result.ComponentBackendType,
			result.ComponentBackendSection,
		)
		if err != nil {
			return nil, err
		}

		// Process remote state backend configuration.
		finalComponentRemoteStateBackendType, finalComponentRemoteStateBackend, err := processTerraformRemoteStateBackend(
			atmosConfig,
			opts.Component,
			finalComponentBackendType,
			map[string]any{finalComponentBackendType: finalComponentBackend},
			opts.GlobalRemoteStateBackendType,
			opts.GlobalRemoteStateBackendSection,
			result.BaseComponentRemoteStateBackendType,
			result.BaseComponentRemoteStateBackendSection,
			result.ComponentRemoteStateBackendType,
			result.ComponentRemoteStateBackendSection,
		)
		if err != nil {
			return nil, err
		}

		// Process auth configuration.
		mergedAuth, err := processAuthConfig(atmosConfig, result.ComponentAuth)
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
					return nil, fmt.Errorf("invalid 'components.terraform.%s.settings.spacelift' section", opts.Component)
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
