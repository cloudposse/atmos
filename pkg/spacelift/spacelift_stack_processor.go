package spacelift

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// CreateSpaceliftStacks takes a list of paths to YAML config files, processes and deep-merges all imports, and returns a map of Spacelift stack configs.
func CreateSpaceliftStacks(
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	packerComponentsBasePath string,
	filePaths []string,
	processStackDeps bool,
	processComponentDeps bool,
	processImports bool,
	stackConfigPathTemplate string,
) (map[string]any, error) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	if err != nil {
		return nil, err
	}

	if len(filePaths) > 0 {
		_, stacks, rawStackConfigs, err := s.ProcessYAMLConfigFiles(
			&atmosConfig,
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			packerComponentsBasePath,
			filePaths,
			processStackDeps,
			processComponentDeps,
			false,
		)
		if err != nil {
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(
			&atmosConfig,
			stacks,
			stackConfigPathTemplate,
			"",
			processImports,
			rawStackConfigs,
		)
	} else {
		_, stacks, rawStackConfigs, err := s.ProcessYAMLConfigFiles(
			&atmosConfig,
			atmosConfig.StacksBaseAbsolutePath,
			atmosConfig.TerraformDirAbsolutePath,
			atmosConfig.HelmfileDirAbsolutePath,
			atmosConfig.PackerDirAbsolutePath,
			atmosConfig.StackConfigFilesAbsolutePaths,
			processStackDeps,
			processComponentDeps,
			false,
		)
		if err != nil {
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(
			&atmosConfig,
			stacks,
			stackConfigPathTemplate,
			e.GetStackNamePattern(&atmosConfig),
			processImports,
			rawStackConfigs,
		)
	}
}

// TransformStackConfigToSpaceliftStacks takes a map of stack manifests and transforms it to a map of Spacelift stacks
func TransformStackConfigToSpaceliftStacks(
	atmosConfig *schema.AtmosConfiguration,
	stacks map[string]any,
	stackConfigPathTemplate string,
	stackNamePattern string,
	processImports bool,
	rawStackConfigs map[string]map[string]any,
) (map[string]any, error) {
	var err error
	res := map[string]any{}

	allStackNames, err := e.BuildSpaceliftStackNames(stacks, stackNamePattern)
	if err != nil {
		return nil, err
	}

	for stackName, stackConfig := range stacks {
		config := stackConfig.(map[string]any)
		var imports []string

		if processImports {
			if i, ok := config["imports"]; ok {
				imports = i.([]string)
			}
		}

		if i, ok := config["components"]; ok {
			componentsSection := i.(map[string]any)

			if terraformComponents, ok := componentsSection["terraform"]; ok {
				terraformComponentsMap := terraformComponents.(map[string]any)

				for component, v := range terraformComponentsMap {
					componentMap := v.(map[string]any)

					componentSettings := map[string]any{}
					if i, ok2 := componentMap["settings"]; ok2 {
						componentSettings = i.(map[string]any)
					}

					spaceliftSettings := map[string]any{}
					spaceliftWorkspaceEnabled := false

					if i, ok2 := componentSettings["spacelift"]; ok2 {
						spaceliftSettings = i.(map[string]any)

						if i3, ok3 := spaceliftSettings["workspace_enabled"]; ok3 {
							spaceliftWorkspaceEnabled = i3.(bool)
						}
					}

					// If Spacelift workspace is disabled, don't include it, continue to the next component
					if !spaceliftWorkspaceEnabled {
						continue
					}

					spaceliftExplicitLabels := []any{}
					if i, ok2 := spaceliftSettings["labels"]; ok2 {
						spaceliftExplicitLabels = i.([]any)
					}

					spaceliftConfig := map[string]any{}
					spaceliftConfig["enabled"] = spaceliftWorkspaceEnabled

					componentVars := map[string]any{}
					if i, ok2 := componentMap["vars"]; ok2 {
						componentVars = i.(map[string]any)
					}

					componentEnv := map[string]any{}
					if i, ok2 := componentMap["env"]; ok2 {
						componentEnv = i.(map[string]any)
					}

					componentStacks := []string{}
					if i, ok2 := componentMap["stacks"]; ok2 {
						componentStacks = i.([]string)
					}

					componentInheritance := []string{}
					if i, ok2 := componentMap["inheritance"]; ok2 {
						componentInheritance = i.([]string)
					}

					// Process component metadata and find a base component (if any) and whether the component is real or abstract
					componentMetadata, baseComponentName, componentIsAbstract, componentIsEnabled, _ := e.ProcessComponentMetadata(component, componentMap)

					if componentIsAbstract || !componentIsEnabled {
						continue
					}

					context := cfg.GetContextFromVars(componentVars)
					context.Component = component
					context.BaseComponent = baseComponentName

					var contextPrefix string

					if stackNamePattern != "" {
						contextPrefix, err = cfg.GetContextPrefix(stackName, context, stackNamePattern, stackName)
						if err != nil {
							return nil, err
						}
					} else {
						contextPrefix = strings.Replace(stackName, "/", "-", -1)
					}

					spaceliftConfig[cfg.ComponentSectionName] = component
					spaceliftConfig["stack"] = contextPrefix
					spaceliftConfig["imports"] = imports
					spaceliftConfig[cfg.VarsSectionName] = componentVars
					spaceliftConfig[cfg.SettingsSectionName] = componentSettings
					spaceliftConfig[cfg.EnvSectionName] = componentEnv
					spaceliftConfig["stacks"] = componentStacks
					spaceliftConfig["inheritance"] = componentInheritance
					spaceliftConfig["base_component"] = baseComponentName
					spaceliftConfig[cfg.MetadataSectionName] = componentMetadata

					// backend
					backendTypeName := ""
					if backendType, backendTypeExist := componentMap["backend_type"]; backendTypeExist {
						backendTypeName = backendType.(string)
					}
					spaceliftConfig["backend_type"] = backendTypeName

					componentBackend := map[string]any{}
					if i, ok2 := componentMap["backend"]; ok2 {
						componentBackend = i.(map[string]any)
					}
					spaceliftConfig["backend"] = componentBackend

					configAndStacksInfo := schema.ConfigAndStacksInfo{
						ComponentFromArg:          component,
						ComponentType:             "terraform",
						StackFile:                 stackName,
						ComponentVarsSection:      componentVars,
						ComponentEnvSection:       componentEnv,
						ComponentSettingsSection:  componentSettings,
						ComponentMetadataSection:  componentMetadata,
						ComponentBackendSection:   componentBackend,
						ComponentBackendType:      backendTypeName,
						ComponentInheritanceChain: componentInheritance,
						Context:                   context,
						ComponentSection: map[string]any{
							cfg.VarsSectionName:        componentVars,
							cfg.EnvSectionName:         componentEnv,
							cfg.SettingsSectionName:    componentSettings,
							cfg.MetadataSectionName:    componentMetadata,
							cfg.BackendSectionName:     componentBackend,
							cfg.BackendTypeSectionName: backendTypeName,
						},
					}

					// Component dependencies
					sources, err := e.ProcessConfigSources(configAndStacksInfo, rawStackConfigs)
					if err != nil {
						return nil, err
					}

					componentDeps, componentDepsAll, err := e.FindComponentDependencies(stackName, sources)
					if err != nil {
						return nil, err
					}

					spaceliftConfig["deps"] = componentDeps
					spaceliftConfig["deps_all"] = componentDepsAll

					// Terraform workspace
					workspace, err := e.BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
					if err != nil {
						return nil, err
					}
					spaceliftConfig["workspace"] = workspace

					// labels
					labels := []string{}
					for _, v := range imports {
						labels = append(labels, fmt.Sprintf("import:"+stackConfigPathTemplate, v))
					}
					for _, v := range componentStacks {
						labels = append(labels, fmt.Sprintf("stack:"+stackConfigPathTemplate, v))
					}
					for _, v := range componentDeps {
						labels = append(labels, fmt.Sprintf("deps:"+stackConfigPathTemplate, v))
					}
					for _, v := range spaceliftExplicitLabels {
						labels = append(labels, v.(string))
					}

					var terraformComponentNamesInCurrentStack []string

					for v2 := range terraformComponentsMap {
						terraformComponentNamesInCurrentStack = append(terraformComponentNamesInCurrentStack, strings.Replace(v2, "/", "-", -1))
					}

					// Legacy/deprecated `settings.spacelift.depends_on`
					spaceliftDependsOn := []any{}
					if i, ok2 := spaceliftSettings["depends_on"]; ok2 {
						spaceliftDependsOn = i.([]any)
					}

					var spaceliftStackNameDependsOnLabels1 []string

					for _, dep := range spaceliftDependsOn {
						spaceliftStackNameDependsOn, err := e.BuildDependentStackNameFromDependsOnLegacy(
							dep.(string),
							allStackNames,
							contextPrefix,
							terraformComponentNamesInCurrentStack,
							component,
						)
						if err != nil {
							return nil, err
						}
						spaceliftStackNameDependsOnLabels1 = append(spaceliftStackNameDependsOnLabels1, fmt.Sprintf("depends-on:%s", spaceliftStackNameDependsOn))
					}

					sort.Strings(spaceliftStackNameDependsOnLabels1)
					labels = append(labels, spaceliftStackNameDependsOnLabels1...)

					// Recommended `settings.depends_on`
					var stackComponentSettingsDependsOn schema.Settings
					err = mapstructure.Decode(componentSettings, &stackComponentSettingsDependsOn)
					if err != nil {
						return nil, err
					}

					var spaceliftStackNameDependsOnLabels2 []string

					for _, stackComponentSettingsDependsOnContext := range stackComponentSettingsDependsOn.DependsOn {
						if stackComponentSettingsDependsOnContext.Component == "" {
							continue
						}

						if stackComponentSettingsDependsOnContext.Namespace == "" {
							stackComponentSettingsDependsOnContext.Namespace = context.Namespace
						}
						if stackComponentSettingsDependsOnContext.Tenant == "" {
							stackComponentSettingsDependsOnContext.Tenant = context.Tenant
						}
						if stackComponentSettingsDependsOnContext.Environment == "" {
							stackComponentSettingsDependsOnContext.Environment = context.Environment
						}
						if stackComponentSettingsDependsOnContext.Stage == "" {
							stackComponentSettingsDependsOnContext.Stage = context.Stage
						}

						var contextPrefixDependsOn string

						if stackNamePattern != "" {
							contextPrefixDependsOn, err = cfg.GetContextPrefix(
								stackName,
								stackComponentSettingsDependsOnContext,
								stackNamePattern,
								stackName,
							)
							if err != nil {
								return nil, err
							}
						} else {
							contextPrefixDependsOn = strings.Replace(stackName, "/", "-", -1)
						}

						spaceliftStackNameDependsOn, err := e.BuildDependentStackNameFromDependsOn(
							component,
							contextPrefix,
							stackComponentSettingsDependsOnContext.Component,
							contextPrefixDependsOn,
							allStackNames,
						)
						if err != nil {
							return nil, err
						}
						spaceliftStackNameDependsOnLabels2 = append(spaceliftStackNameDependsOnLabels2, fmt.Sprintf("depends-on:%s", spaceliftStackNameDependsOn))
					}

					sort.Strings(spaceliftStackNameDependsOnLabels2)
					labels = append(labels, spaceliftStackNameDependsOnLabels2...)

					// Add `component` and `folder` labels
					labels = append(labels, fmt.Sprintf("folder:component/%s", component))
					labels = append(labels, fmt.Sprintf("folder:%s", strings.Replace(contextPrefix, "-", "/", -1)))

					spaceliftConfig["labels"] = u.UniqueStrings(labels)

					// Spacelift stack name
					spaceliftStackName, spaceliftStackNamePattern, err := e.BuildSpaceliftStackName(spaceliftSettings, context, contextPrefix)
					if err != nil {
						return nil, err
					}

					// Add Spacelift stack config to the final map
					spaceliftStackNameKey := strings.Replace(spaceliftStackName, "/", "-", -1)

					if !u.MapKeyExists(res, spaceliftStackNameKey) {
						res[spaceliftStackNameKey] = spaceliftConfig
					} else {
						errorMessage := fmt.Sprintf("\nDuplicate Spacelift stack name '%s' for component '%s' in the stack '%s'."+
							"\nCheck if the component name is correct and the Spacelift stack name pattern 'stack_name_pattern=%s' is specific enough."+
							"\nDid you specify the correct context tokens {namespace}, {tenant}, {environment}, {stage}, {component}?",
							spaceliftStackName,
							component,
							stackName,
							spaceliftStackNamePattern,
						)
						er := errors.New(errorMessage)
						return nil, er
					}
				}
			}
		}
	}

	return res, nil
}
