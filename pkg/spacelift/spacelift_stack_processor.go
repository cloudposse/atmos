package spacelift

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// CreateSpaceliftStacks takes a list of paths to YAML config files, processes and deep-merges all imports,
// and returns a map of Spacelift stack configs
func CreateSpaceliftStacks(
	stacksBasePath string,
	terraformComponentsBasePath string,
	helmfileComponentsBasePath string,
	filePaths []string,
	processStackDeps bool,
	processComponentDeps bool,
	processImports bool,
	stackConfigPathTemplate string,
) (map[string]any, error) {

	if len(filePaths) > 0 {
		_, stacks, rawStackConfigs, err := s.ProcessYAMLConfigFiles(
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			filePaths,
			processStackDeps,
			processComponentDeps,
			false,
		)
		if err != nil {
			u.LogError(err)
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(stacks, stackConfigPathTemplate, "", processImports, rawStackConfigs)
	} else {
		cliConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			u.LogError(err)
			return nil, err
		}

		_, stacks, rawStackConfigs, err := s.ProcessYAMLConfigFiles(
			cliConfig.StacksBaseAbsolutePath,
			cliConfig.TerraformDirAbsolutePath,
			cliConfig.HelmfileDirAbsolutePath,
			cliConfig.StackConfigFilesAbsolutePaths,
			processStackDeps,
			processComponentDeps,
			false,
		)
		if err != nil {
			u.LogError(err)
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(stacks, stackConfigPathTemplate, cliConfig.Stacks.NamePattern, processImports, rawStackConfigs)
	}
}

// TransformStackConfigToSpaceliftStacks takes a map of stack configs and transforms it to a map of Spacelift stacks
func TransformStackConfigToSpaceliftStacks(
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
		config := stackConfig.(map[any]any)
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

					componentSettings := map[any]any{}
					if i, ok2 := componentMap["settings"]; ok2 {
						componentSettings = i.(map[any]any)
					}

					spaceliftSettings := map[any]any{}
					spaceliftWorkspaceEnabled := false

					if i, ok2 := componentSettings["spacelift"]; ok2 {
						spaceliftSettings = i.(map[any]any)

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

					componentVars := map[any]any{}
					if i, ok2 := componentMap["vars"]; ok2 {
						componentVars = i.(map[any]any)
					}

					componentEnv := map[any]any{}
					if i, ok2 := componentMap["env"]; ok2 {
						componentEnv = i.(map[any]any)
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
					componentMetadata, baseComponentName, componentIsAbstract := e.ProcessComponentMetadata(component, componentMap)

					if componentIsAbstract {
						continue
					}

					context := cfg.GetContextFromVars(componentVars)
					context.Component = component
					context.BaseComponent = baseComponentName

					var contextPrefix string

					if stackNamePattern != "" {
						contextPrefix, err = cfg.GetContextPrefix(stackName, context, stackNamePattern, stackName)
						if err != nil {
							u.LogError(err)
							return nil, err
						}
					} else {
						contextPrefix = strings.Replace(stackName, "/", "-", -1)
					}

					spaceliftConfig["component"] = component
					spaceliftConfig["stack"] = contextPrefix
					spaceliftConfig["imports"] = imports
					spaceliftConfig["vars"] = componentVars
					spaceliftConfig["settings"] = componentSettings
					spaceliftConfig["env"] = componentEnv
					spaceliftConfig["stacks"] = componentStacks
					spaceliftConfig["inheritance"] = componentInheritance
					spaceliftConfig["base_component"] = baseComponentName
					spaceliftConfig["metadata"] = componentMetadata

					// backend
					backendTypeName := ""
					if backendType, backendTypeExist := componentMap["backend_type"]; backendTypeExist {
						backendTypeName = backendType.(string)
					}
					spaceliftConfig["backend_type"] = backendTypeName

					componentBackend := map[any]any{}
					if i, ok2 := componentMap["backend"]; ok2 {
						componentBackend = i.(map[any]any)
					}
					spaceliftConfig["backend"] = componentBackend

					// Component dependencies
					configAndStacksInfo := schema.ConfigAndStacksInfo{
						ComponentFromArg:          component,
						ComponentType:             "terraform",
						StackFile:                 stackName,
						ComponentVarsSection:      componentVars,
						ComponentEnvSection:       componentEnv,
						ComponentSettingsSection:  componentSettings,
						ComponentBackendSection:   componentBackend,
						ComponentBackendType:      backendTypeName,
						ComponentInheritanceChain: componentInheritance,
					}

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
					workspace, err := e.BuildTerraformWorkspace(
						stackName,
						stackNamePattern,
						componentMetadata,
						context,
					)
					if err != nil {
						u.LogError(err)
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

					for v := range terraformComponentsMap {
						terraformComponentNamesInCurrentStack = append(terraformComponentNamesInCurrentStack, strings.Replace(v, "/", "-", -1))
					}

					// Legacy/deprecated `settings.spacelift.depends_on`
					spaceliftDependsOn := []any{}
					if i, ok2 := spaceliftSettings["depends_on"]; ok2 {
						spaceliftDependsOn = i.([]any)
					}

					// Recommended `settings.depends_on`
					componentDependsOn := map[string]any{}
					if i, ok2 := componentSettings["depends_on"]; ok2 {
						componentDependsOn = i.(map[string]any)
					}

					for _, dep := range spaceliftDependsOn {
						spaceliftStackNameDependsOn, err := e.BuildDependentStackNameFromDependsOn(
							dep.(string),
							allStackNames,
							contextPrefix,
							terraformComponentNamesInCurrentStack,
							component)
						if err != nil {
							u.LogError(err)
							return nil, err
						}
						labels = append(labels, fmt.Sprintf("depends-on:%s", spaceliftStackNameDependsOn))
					}

					// Add `component` and `folder` labels
					labels = append(labels, fmt.Sprintf("folder:component/%s", component))
					labels = append(labels, fmt.Sprintf("folder:%s", strings.Replace(contextPrefix, "-", "/", -1)))

					spaceliftConfig["labels"] = u.UniqueStrings(labels)

					// Spacelift stack name
					spaceliftStackName, spaceliftStackNamePattern := e.BuildSpaceliftStackName(spaceliftSettings, context, contextPrefix)

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
						u.LogError(er)
						return nil, er
					}
				}
			}
		}
	}

	return res, nil
}
