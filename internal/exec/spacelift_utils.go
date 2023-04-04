package exec

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// BuildSpaceliftStackName builds a Spacelift stack name from the provided context and stack name pattern
func BuildSpaceliftStackName(spaceliftSettings map[any]any, context schema.Context, contextPrefix string) (string, string) {
	if spaceliftStackNamePattern, ok := spaceliftSettings["stack_name_pattern"].(string); ok {
		return cfg.ReplaceContextTokens(context, spaceliftStackNamePattern), spaceliftStackNamePattern
	} else if spaceliftStackName, ok := spaceliftSettings["stack_name"].(string); ok {
		return spaceliftStackName, contextPrefix
	} else {
		defaultSpaceliftStackNamePattern := fmt.Sprintf("%s-%s", contextPrefix, context.Component)
		return strings.Replace(defaultSpaceliftStackNamePattern, "/", "-", -1), contextPrefix
	}
}

// BuildSpaceliftStackNames builds Spacelift stack names
func BuildSpaceliftStackNames(stacks map[string]any, stackNamePattern string) ([]string, error) {
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

					context := cfg.GetContextFromVars(componentVars)

					var contextPrefix string
					var err error

					if stackNamePattern != "" {
						contextPrefix, err = cfg.GetContextPrefix(stackName, context, stackNamePattern, stackName)
						if err != nil {
							u.LogErrorToStdError(err)
							return nil, err
						}
					} else {
						contextPrefix = strings.Replace(stackName, "/", "-", -1)
					}

					context.Component = component
					spaceliftStackName, _ := BuildSpaceliftStackName(spaceliftSettings, context, contextPrefix)
					allStackNames = append(allStackNames, strings.Replace(spaceliftStackName, "/", "-", -1))
				}
			}
		}
	}

	return allStackNames, nil
}

// BuildSpaceliftStackNameFromComponentConfig builds Spacelift stack name from the component config
func BuildSpaceliftStackNameFromComponentConfig(
	cliConfig schema.CliConfiguration,
	componentName string,
	stackName string,
	componentSettingsSection map[any]any,
	componentVarsSection map[any]any,
) (string, error) {

	var spaceliftStackName string
	var spaceliftSettingsSection map[any]any

	if i, ok2 := componentSettingsSection["spacelift"]; ok2 {
		spaceliftSettingsSection = i.(map[any]any)
	}

	context := cfg.GetContextFromVars(componentVarsSection)
	context.Component = strings.Replace(componentName, "/", "-", -1)

	// Spacelift stack
	if spaceliftWorkspaceEnabled, ok := spaceliftSettingsSection["workspace_enabled"].(bool); ok && spaceliftWorkspaceEnabled {
		contextPrefix, err := cfg.GetContextPrefix(stackName, context, cliConfig.Stacks.NamePattern, stackName)
		if err != nil {
			return "", err
		}

		spaceliftStackName, _ = BuildSpaceliftStackName(spaceliftSettingsSection, context, contextPrefix)
	}

	return spaceliftStackName, nil
}
