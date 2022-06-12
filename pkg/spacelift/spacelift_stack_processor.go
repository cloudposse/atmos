package spacelift

import (
	"fmt"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
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
	stackConfigPathTemplate string) (map[string]any, error) {

	if len(filePaths) > 0 {
		_, stacks, err := s.ProcessYAMLConfigFiles(
			stacksBasePath,
			terraformComponentsBasePath,
			helmfileComponentsBasePath,
			filePaths,
			processStackDeps,
			processComponentDeps,
		)
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(stacks, stackConfigPathTemplate, "", processImports)
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
			c.ProcessedConfig.TerraformDirAbsolutePath,
			c.ProcessedConfig.HelmfileDirAbsolutePath,
			c.ProcessedConfig.StackConfigFilesAbsolutePaths,
			processStackDeps,
			processComponentDeps,
		)
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}

		return TransformStackConfigToSpaceliftStacks(stacks, stackConfigPathTemplate, c.Config.Stacks.NamePattern, processImports)
	}
}

// TransformStackConfigToSpaceliftStacks takes a map of stack configs and transforms it to a map of Spacelift stacks
func TransformStackConfigToSpaceliftStacks(
	stacks map[string]any,
	stackConfigPathTemplate string,
	stackNamePattern string,
	processImports bool) (map[string]any, error) {

	var err error
	res := map[string]any{}
	var allStackNames []string

	for stackName, stackConfig := range stacks {
		config := stackConfig.(map[any]any)

		if i, ok := config["components"]; ok {
			componentsSection := i.(map[string]any)

			if terraformComponents, ok := componentsSection["terraform"]; ok {
				terraformComponentsMap := terraformComponents.(map[string]any)

				for component, v := range terraformComponentsMap {
					componentMap := v.(map[string]any)
					componentVars := map[any]any{}
					spaceliftSettings := map[any]any{}

					if i, ok2 := componentMap["vars"]; ok2 {
						componentVars = i.(map[any]any)
					}

					componentSettings := map[any]any{}
					if i, ok2 := componentMap["settings"]; ok2 {
						componentSettings = i.(map[any]any)
					}

					if i, ok2 := componentSettings["spacelift"]; ok2 {
						spaceliftSettings = i.(map[any]any)
					}

					context := c.GetContextFromVars(componentVars)

					var contextPrefix string

					if stackNamePattern != "" {
						contextPrefix, err = c.GetContextPrefix(stackName, context, stackNamePattern)
						if err != nil {
							u.PrintErrorToStdError(err)
							return nil, err
						}
					} else {
						contextPrefix = strings.Replace(stackName, "/", "-", -1)
					}

					context.Component = component
					spaceliftStackName, _ := buildSpaceliftStackName(spaceliftSettings, context, contextPrefix)
					allStackNames = append(allStackNames, strings.Replace(spaceliftStackName, "/", "-", -1))
				}
			}
		}
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

					spaceliftDependsOn := []any{}
					if i, ok2 := spaceliftSettings["depends_on"]; ok2 {
						spaceliftDependsOn = i.([]any)
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
					if componentMetadata, componentMetadataExists := componentMap["metadata"].(map[any]any); componentMetadataExists {
						if componentFromMetadata, componentFromMetadataExists := componentMetadata["component"].(string); componentFromMetadataExists {
							baseComponentName = componentFromMetadata
						}
					}

					context := c.GetContextFromVars(componentVars)
					context.Component = component
					context.BaseComponent = baseComponentName

					var contextPrefix string

					if stackNamePattern != "" {
						contextPrefix, err = c.GetContextPrefix(stackName, context, stackNamePattern)
						if err != nil {
							u.PrintErrorToStdError(err)
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

					componentBackend := map[any]any{}
					if i, ok2 := componentMap["backend"]; ok2 {
						componentBackend = i.(map[any]any)
					}
					spaceliftConfig["backend"] = componentBackend

					// metadata
					componentMetadata := map[any]any{}
					if i, ok2 := componentMap["metadata"]; ok2 {
						componentMetadata = i.(map[any]any)
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
		errorMessage := fmt.Errorf("component '%[1]s' in stack '%[2]s' specifies 'depends_on' dependency '%[3]s', "+
			"but '%[3]s' is not a stack and not a terraform component in '%[2]s' stack",
			currentComponentName,
			currentStackName,
			dependsOn)

		return "", errorMessage
	}

	return spaceliftStackName, nil
}

// buildSpaceliftStackName build a Spacelift stack name from the provided context and state name pattern
func buildSpaceliftStackName(spaceliftSettings map[any]any, context c.Context, contextPrefix string) (string, string) {
	if spaceliftStackNamePattern, ok := spaceliftSettings["stack_name_pattern"].(string); ok {
		return c.ReplaceContextTokens(context, spaceliftStackNamePattern), spaceliftStackNamePattern
	} else {
		defaultSpaceliftStackNamePattern := fmt.Sprintf("%s-%s", contextPrefix, context.Component)
		return strings.Replace(defaultSpaceliftStackNamePattern, "/", "-", -1), contextPrefix
	}
}
