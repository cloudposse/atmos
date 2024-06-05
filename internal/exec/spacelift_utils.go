package exec

import (
	"fmt"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// BuildSpaceliftStackName builds a Spacelift stack name from the provided context and stack name pattern
func BuildSpaceliftStackName(spaceliftSettings map[any]any, context schema.Context, contextPrefix string) (string, string, error) {
	if spaceliftStackNamePattern, ok := spaceliftSettings["stack_name_pattern"].(string); ok {
		return cfg.ReplaceContextTokens(context, spaceliftStackNamePattern), spaceliftStackNamePattern, nil
	} else if spaceliftStackName, ok := spaceliftSettings["stack_name"].(string); ok {
		return spaceliftStackName, contextPrefix, nil
	} else {
		defaultSpaceliftStackNamePattern := fmt.Sprintf("%s-%s", contextPrefix, context.Component)
		return strings.Replace(defaultSpaceliftStackNamePattern, "/", "-", -1), contextPrefix, nil
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
							return nil, err
						}
					} else {
						contextPrefix = strings.Replace(stackName, "/", "-", -1)
					}

					context.Component = component

					spaceliftStackName, _, err := BuildSpaceliftStackName(spaceliftSettings, context, contextPrefix)
					if err != nil {
						return nil, err
					}

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
	configAndStacksInfo schema.ConfigAndStacksInfo,
) (string, error) {

	var spaceliftStackName string
	var spaceliftSettingsSection map[any]any
	var contextPrefix string
	var err error

	if i, ok2 := configAndStacksInfo.ComponentSettingsSection["spacelift"]; ok2 {
		spaceliftSettingsSection = i.(map[any]any)
	}

	// Spacelift stack
	if spaceliftWorkspaceEnabled, ok := spaceliftSettingsSection["workspace_enabled"].(bool); ok && spaceliftWorkspaceEnabled {
		context := cfg.GetContextFromVars(configAndStacksInfo.ComponentVarsSection)
		context.Component = strings.Replace(configAndStacksInfo.ComponentFromArg, "/", "-", -1)

		if cliConfig.Stacks.NameTemplate != "" {
			contextPrefix, err = u.ProcessTmpl("name-template", cliConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, false)
			if err != nil {
				return "", err
			}
		} else {
			contextPrefix, err = cfg.GetContextPrefix(configAndStacksInfo.Stack, context, GetStackNamePattern(cliConfig), configAndStacksInfo.Stack)
			if err != nil {
				return "", err
			}
		}

		spaceliftStackName, _, err = BuildSpaceliftStackName(spaceliftSettingsSection, context, contextPrefix)
		if err != nil {
			return "", err
		}
	}

	return spaceliftStackName, nil
}
