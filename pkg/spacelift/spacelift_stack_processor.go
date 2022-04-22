package spacelift

import (
	"fmt"
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
	"strings"
)

// CreateSpaceliftStacks takes a list of paths to YAML config files, processes and deep-merges all imports,
// and returns a map of Spacelift stack configs
func CreateSpaceliftStacks(
	basePath string,
	filePaths []string,
	processStackDeps bool,
	processComponentDeps bool,
	processImports bool,
	stackConfigPathTemplate string) (map[string]interface{}, error) {

	if filePaths != nil && len(filePaths) > 0 {
		_, stacks, err := s.ProcessYAMLConfigFiles(basePath, filePaths, processStackDeps, processComponentDeps)
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}

		return LegacyTransformStackConfigToSpaceliftStacks(stacks, stackConfigPathTemplate, processImports)
	} else {
		err := c.InitConfig()
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}

		err = c.ProcessConfigForSpacelift()
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}

		_, stacks, err := s.ProcessYAMLConfigFiles(
			c.ProcessedConfig.StacksBaseAbsolutePath,
			c.ProcessedConfig.StackConfigFilesAbsolutePaths,
			processStackDeps,
			processComponentDeps)
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(stacks, stackConfigPathTemplate, c.Config.Stacks.NamePattern, processImports)
	}
}

// LegacyTransformStackConfigToSpaceliftStacks takes a map of stack configs and transforms it to a map of Spacelift stacks
func LegacyTransformStackConfigToSpaceliftStacks(
	stacks map[string]interface{},
	stackConfigPathTemplate string,
	processImports bool) (map[string]interface{}, error) {

	res := map[string]interface{}{}

	var allStackNames []string
	for stack, stackConfig := range stacks {
		config := stackConfig.(map[interface{}]interface{})

		if i, ok := config["components"]; ok {
			componentsSection := i.(map[string]interface{})

			if terraformComponents, ok := componentsSection["terraform"]; ok {
				terraformComponentsMap := terraformComponents.(map[string]interface{})

				for component, _ := range terraformComponentsMap {
					allStackNames = append(allStackNames, fmt.Sprintf("%s-%s", stack, component))
				}
			}
		}
	}

	for stackName, stackConfig := range stacks {
		config := stackConfig.(map[interface{}]interface{})
		var imports []string

		if processImports == true {
			if i, ok := config["imports"]; ok {
				imports = i.([]string)
			}
		}

		if i, ok := config["components"]; ok {
			componentsSection := i.(map[string]interface{})

			if terraformComponents, ok := componentsSection["terraform"]; ok {
				terraformComponentsMap := terraformComponents.(map[string]interface{})

				terraformComponentNamesInCurrentStack := u.StringKeysFromMap(terraformComponentsMap)

				for component, v := range terraformComponentsMap {
					componentMap := v.(map[string]interface{})

					componentSettings := map[interface{}]interface{}{}
					if i, ok2 := componentMap["settings"]; ok2 {
						componentSettings = i.(map[interface{}]interface{})
					}

					spaceliftSettings := map[interface{}]interface{}{}
					spaceliftWorkspaceEnabled := false

					if i, ok2 := componentSettings["spacelift"]; ok2 {
						spaceliftSettings = i.(map[interface{}]interface{})

						if i3, ok3 := spaceliftSettings["workspace_enabled"]; ok3 {
							spaceliftWorkspaceEnabled = i3.(bool)
						}
					}

					// If Spacelift workspace is disabled, don't include it, continue to the next component
					if spaceliftWorkspaceEnabled == false {
						continue
					}

					spaceliftExplicitLabels := []interface{}{}
					if i, ok2 := spaceliftSettings["labels"]; ok2 {
						spaceliftExplicitLabels = i.([]interface{})
					}

					spaceliftDependsOn := []interface{}{}
					if i, ok2 := spaceliftSettings["depends_on"]; ok2 {
						spaceliftDependsOn = i.([]interface{})
					}

					spaceliftConfig := map[string]interface{}{}
					spaceliftConfig["enabled"] = spaceliftWorkspaceEnabled

					componentVars := map[interface{}]interface{}{}
					if i, ok2 := componentMap["vars"]; ok2 {
						componentVars = i.(map[interface{}]interface{})
					}

					componentEnv := map[interface{}]interface{}{}
					if i, ok2 := componentMap["env"]; ok2 {
						componentEnv = i.(map[interface{}]interface{})
					}

					componentDeps := []string{}
					if i, ok2 := componentMap["deps"]; ok2 {
						componentDeps = i.([]string)
					}

					componentStacks := []string{}
					if i, ok2 := componentMap["stacks"]; ok2 {
						componentStacks = i.([]string)
					}

					spaceliftConfig["component"] = component
					spaceliftConfig["stack"] = stackName
					spaceliftConfig["imports"] = imports
					spaceliftConfig["vars"] = componentVars
					spaceliftConfig["settings"] = componentSettings
					spaceliftConfig["env"] = componentEnv
					spaceliftConfig["deps"] = componentDeps
					spaceliftConfig["stacks"] = componentStacks

					baseComponentName := ""
					if baseComponent, baseComponentExist := componentMap["component"]; baseComponentExist {
						baseComponentName = baseComponent.(string)
					}
					spaceliftConfig["base_component"] = baseComponentName

					// backend
					backendTypeName := ""
					if backendType, backendTypeExist := componentMap["backend_type"]; backendTypeExist {
						backendTypeName = backendType.(string)
					}
					spaceliftConfig["backend_type"] = backendTypeName

					componentBackend := map[interface{}]interface{}{}
					if i, ok2 := componentMap["backend"]; ok2 {
						componentBackend = i.(map[interface{}]interface{})
					}
					spaceliftConfig["backend"] = componentBackend

					// workspace
					var workspace string
					if backendTypeName == "s3" && baseComponentName == "" {
						workspace = stackName
					} else {
						workspace = fmt.Sprintf("%s-%s", stackName, component)
					}
					spaceliftConfig["workspace"] = strings.Replace(workspace, "/", "-", -1)

					// labels
					var labels []string
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
					for _, v := range spaceliftDependsOn {
						spaceliftStackName, err := buildSpaceliftDependsOnStackName(
							v.(string),
							allStackNames,
							stackName,
							terraformComponentNamesInCurrentStack,
							component)
						if err != nil {
							u.PrintErrorToStdError(err)
							return nil, err
						}
						labels = append(labels, fmt.Sprintf("depends-on:%s", spaceliftStackName))
					}

					labels = append(labels, fmt.Sprintf("folder:component/%s", component))

					// Split on the first `-` and get the two parts: environment and stage
					stackNameParts := strings.SplitN(stackName, "-", 2)
					stackNamePartsLen := len(stackNameParts)
					if stackNamePartsLen == 2 {
						labels = append(labels, fmt.Sprintf("folder:%s/%s", stackNameParts[0], stackNameParts[1]))
					}

					spaceliftConfig["labels"] = u.UniqueStrings(labels)

					// Add Spacelift stack config to the final map
					spaceliftStackName := strings.Replace(fmt.Sprintf("%s-%s", stackName, component), "/", "-", -1)
					res[spaceliftStackName] = spaceliftConfig
				}
			}
		}
	}

	return res, nil
}

// TransformStackConfigToSpaceliftStacks takes a map of stack configs and transforms it to a map of Spacelift stacks
func TransformStackConfigToSpaceliftStacks(
	stacks map[string]interface{},
	stackConfigPathTemplate string,
	stackNamePattern string,
	processImports bool) (map[string]interface{}, error) {

	res := map[string]interface{}{}
	var allStackNames []string

	for stackName, stackConfig := range stacks {
		config := stackConfig.(map[interface{}]interface{})

		if i, ok := config["components"]; ok {
			componentsSection := i.(map[string]interface{})

			if terraformComponents, ok := componentsSection["terraform"]; ok {
				terraformComponentsMap := terraformComponents.(map[string]interface{})

				for component, v := range terraformComponentsMap {
					componentMap := v.(map[string]interface{})
					componentVars := map[interface{}]interface{}{}
					spaceliftSettings := map[interface{}]interface{}{}

					if i, ok2 := componentMap["vars"]; ok2 {
						componentVars = i.(map[interface{}]interface{})
					}

					componentSettings := map[interface{}]interface{}{}
					if i, ok2 := componentMap["settings"]; ok2 {
						componentSettings = i.(map[interface{}]interface{})
					}

					if i, ok2 := componentSettings["spacelift"]; ok2 {
						spaceliftSettings = i.(map[interface{}]interface{})
					}

					// Spacelift stack name
					context := c.GetContextFromVars(componentVars)
					contextPrefix, err := c.GetContextPrefix(stackName, context, stackNamePattern)
					if err != nil {
						u.PrintErrorToStdError(err)
						return nil, err
					}

					context.Component = component
					spaceliftStackName, _ := buildSpaceliftStackName(spaceliftSettings, context, contextPrefix)
					allStackNames = append(allStackNames, strings.Replace(spaceliftStackName, "/", "-", -1))
				}
			}
		}
	}

	for stackName, stackConfig := range stacks {
		config := stackConfig.(map[interface{}]interface{})
		var imports []string

		if processImports == true {
			if i, ok := config["imports"]; ok {
				imports = i.([]string)
			}
		}

		if i, ok := config["components"]; ok {
			componentsSection := i.(map[string]interface{})

			if terraformComponents, ok := componentsSection["terraform"]; ok {
				terraformComponentsMap := terraformComponents.(map[string]interface{})

				for component, v := range terraformComponentsMap {
					componentMap := v.(map[string]interface{})

					componentSettings := map[interface{}]interface{}{}
					if i, ok2 := componentMap["settings"]; ok2 {
						componentSettings = i.(map[interface{}]interface{})
					}

					spaceliftSettings := map[interface{}]interface{}{}
					spaceliftWorkspaceEnabled := false

					if i, ok2 := componentSettings["spacelift"]; ok2 {
						spaceliftSettings = i.(map[interface{}]interface{})

						if i3, ok3 := spaceliftSettings["workspace_enabled"]; ok3 {
							spaceliftWorkspaceEnabled = i3.(bool)
						}
					}

					// If Spacelift workspace is disabled, don't include it, continue to the next component
					if spaceliftWorkspaceEnabled == false {
						continue
					}

					spaceliftExplicitLabels := []interface{}{}
					if i, ok2 := spaceliftSettings["labels"]; ok2 {
						spaceliftExplicitLabels = i.([]interface{})
					}

					spaceliftDependsOn := []interface{}{}
					if i, ok2 := spaceliftSettings["depends_on"]; ok2 {
						spaceliftDependsOn = i.([]interface{})
					}

					spaceliftConfig := map[string]interface{}{}
					spaceliftConfig["enabled"] = spaceliftWorkspaceEnabled

					componentVars := map[interface{}]interface{}{}
					if i, ok2 := componentMap["vars"]; ok2 {
						componentVars = i.(map[interface{}]interface{})
					}

					componentEnv := map[interface{}]interface{}{}
					if i, ok2 := componentMap["env"]; ok2 {
						componentEnv = i.(map[interface{}]interface{})
					}

					componentDeps := []string{}
					if i, ok2 := componentMap["deps"]; ok2 {
						componentDeps = i.([]string)
					}

					componentStacks := []string{}
					if i, ok2 := componentMap["stacks"]; ok2 {
						componentStacks = i.([]string)
					}

					componentInheritance := []string{}
					if i, ok2 := componentMap["inheritance"]; ok2 {
						componentInheritance = i.([]string)
					}

					// Process base component
					// Base component can be specified in two places:
					// `component` attribute (legacy)
					// `metadata.component` attribute
					// `metadata.component` takes precedence over `component`
					baseComponentName := ""
					if baseComponent, baseComponentExist := componentMap["component"]; baseComponentExist {
						baseComponentName = baseComponent.(string)
					}
					// First check if component's `metadata` section exists
					// Then check if `metadata.component` exists
					if componentMetadata, componentMetadataExists := componentMap["metadata"].(map[interface{}]interface{}); componentMetadataExists {
						if componentFromMetadata, componentFromMetadataExists := componentMetadata["component"].(string); componentFromMetadataExists {
							baseComponentName = componentFromMetadata
						}
					}

					context := c.GetContextFromVars(componentVars)
					context.Component = component
					context.BaseComponent = baseComponentName

					contextPrefix, err := c.GetContextPrefix(stackName, context, stackNamePattern)
					if err != nil {
						u.PrintErrorToStdError(err)
						return nil, err
					}

					spaceliftConfig["component"] = component
					spaceliftConfig["stack"] = contextPrefix
					spaceliftConfig["imports"] = imports
					spaceliftConfig["vars"] = componentVars
					spaceliftConfig["settings"] = componentSettings
					spaceliftConfig["env"] = componentEnv
					spaceliftConfig["deps"] = componentDeps
					spaceliftConfig["stacks"] = componentStacks
					spaceliftConfig["inheritance"] = componentInheritance
					spaceliftConfig["base_component"] = baseComponentName

					// backend
					backendTypeName := ""
					if backendType, backendTypeExist := componentMap["backend_type"]; backendTypeExist {
						backendTypeName = backendType.(string)
					}
					spaceliftConfig["backend_type"] = backendTypeName

					componentBackend := map[interface{}]interface{}{}
					if i, ok2 := componentMap["backend"]; ok2 {
						componentBackend = i.(map[interface{}]interface{})
					}
					spaceliftConfig["backend"] = componentBackend

					// metadata
					componentMetadata := map[interface{}]interface{}{}
					if i, ok2 := componentMap["metadata"]; ok2 {
						componentMetadata = i.(map[interface{}]interface{})
					}
					spaceliftConfig["metadata"] = componentMetadata

					// workspace
					workspace, err := e.BuildTerraformWorkspace(
						stackName,
						stackNamePattern,
						componentMetadata,
						context,
					)
					if err != nil {
						u.PrintErrorToStdError(err)
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

					for _, v := range spaceliftDependsOn {
						spaceliftStackNameDependsOn, err := buildSpaceliftDependsOnStackName(
							v.(string),
							allStackNames,
							contextPrefix,
							terraformComponentNamesInCurrentStack,
							component)
						if err != nil {
							u.PrintErrorToStdError(err)
							return nil, err
						}
						labels = append(labels, fmt.Sprintf("depends-on:%s", spaceliftStackNameDependsOn))
					}

					labels = append(labels, fmt.Sprintf("folder:component/%s", component))
					labels = append(labels, fmt.Sprintf("folder:%s", strings.Replace(contextPrefix, "-", "/", -1)))

					spaceliftConfig["labels"] = u.UniqueStrings(labels)

					// Spacelift stack name
					spaceliftStackName, spaceliftStackNamePattern := buildSpaceliftStackName(spaceliftSettings, context, contextPrefix)

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
						u.PrintErrorToStdError(er)
						return nil, er
					}
				}
			}
		}
	}

	return res, nil
}

func buildSpaceliftDependsOnStackName(
	dependsOn string,
	allStackNames []string,
	currentStackName string,
	componentNamesInCurrentStack []string,
	currentComponentName string,
) (string, error) {
	var spaceliftStackName string

	if u.SliceContainsString(allStackNames, dependsOn) {
		spaceliftStackName = dependsOn
	} else if u.SliceContainsString(componentNamesInCurrentStack, dependsOn) {
		spaceliftStackName = fmt.Sprintf("%s-%s", currentStackName, dependsOn)
	} else {
		errorMessage := errors.New(fmt.Sprintf("Component '%[1]s' in stack '%[2]s' specifies 'depends_on' dependency '%[3]s', "+
			"but '%[3]s' is not a stack and not a terraform component in '%[2]s' stack",
			currentComponentName,
			currentStackName,
			dependsOn))

		return "", errorMessage
	}

	return spaceliftStackName, nil
}

// buildSpaceliftStackName build a Spacelift stack name from the provided context and state name pattern
func buildSpaceliftStackName(spaceliftSettings map[interface{}]interface{}, context c.Context, contextPrefix string) (string, string) {
	if spaceliftStackNamePattern, ok := spaceliftSettings["stack_name_pattern"].(string); ok {
		return c.ReplaceContextTokens(context, spaceliftStackNamePattern), spaceliftStackNamePattern
	} else {
		defaultSpaceliftStackNamePattern := fmt.Sprintf("%s-%s", contextPrefix, context.Component)
		return strings.Replace(defaultSpaceliftStackNamePattern, "/", "-", -1), contextPrefix
	}
}
