package component

import (
	"fmt"
	"github.com/cloudposse/atmos/pkg/config"
	s "github.com/cloudposse/atmos/pkg/stack"
	"github.com/pkg/errors"
	"strings"
)

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack
func ProcessComponentInStack(component string, stack string) (map[string]interface{}, error) {
	return ProcessComponentInStackWithPath(component, stack, "")
}

func ProcessComponentInStackWithPath(component string, stack string, basePath string) (map[string]interface{}, error) {
	var configAndStacksInfo config.ConfigAndStacksInfo
	configAndStacksInfo.Stack = stack

	if len(basePath) > 0 {
		configAndStacksInfo.ConfigDir = basePath
	}

	err := config.InitConfig()
	if err != nil {
		return nil, err
	}

	err = config.ProcessConfig(configAndStacksInfo)
	if err != nil {
		return nil, err
	}

	_, stacksMap, err := s.ProcessYAMLConfigFiles(
		config.ProcessedConfig.StacksBaseAbsolutePath,
		config.ProcessedConfig.StackConfigFilesAbsolutePaths,
		true,
		true)

	if err != nil {
		return nil, err
	}

	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}

	// Check and process stacks
	if config.ProcessedConfig.StackType == "Directory" {
		componentSection, componentVarsSection, _, err = findComponentConfig(stack, stacksMap, "terraform", component)
		if err != nil {
			componentSection, componentVarsSection, _, err = findComponentConfig(stack, stacksMap, "helmfile", component)
			if err != nil {
				return nil, err
			}
		}
	} else {
		if len(config.Config.Stacks.NamePattern) < 1 {
			return nil, errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		}

		stackParts := strings.Split(stack, "-")
		stackNamePatternParts := strings.Split(config.Config.Stacks.NamePattern, "-")

		var tenant string
		var environment string
		var stage string
		var tenantFound bool
		var environmentFound bool
		var stageFound bool

		for i, part := range stackNamePatternParts {
			if part == "{tenant}" {
				tenant = stackParts[i]
			} else if part == "{environment}" {
				environment = stackParts[i]
			} else if part == "{stage}" {
				stage = stackParts[i]
			}
		}

		for stackName := range stacksMap {
			componentSection, componentVarsSection, _, err = findComponentConfig(stackName, stacksMap, "terraform", component)
			if err != nil {
				componentSection, componentVarsSection, _, err = findComponentConfig(stackName, stacksMap, "helmfile", component)
				if err != nil {
					continue
				}
			}

			tenantFound = true
			environmentFound = true
			stageFound = true

			// Search for tenant in stack
			if len(tenant) > 0 {
				if tenantInStack, ok := componentVarsSection["tenant"].(string); !ok || tenantInStack != tenant {
					tenantFound = false
				}
			}

			// Search for environment in stack
			if len(environment) > 0 {
				if environmentInStack, ok := componentVarsSection["environment"].(string); !ok || environmentInStack != environment {
					environmentFound = false
				}
			}

			// Search for stage in stack
			if len(stage) > 0 {
				if stageInStack, ok := componentVarsSection["stage"].(string); !ok || stageInStack != stage {
					stageFound = false
				}
			}

			if tenantFound == true && environmentFound == true && stageFound == true {
				break
			}
		}

		if tenantFound == false || environmentFound == false || stageFound == false {
			return nil, errors.New(fmt.Sprintf("\nCould not find config for the component '%s' in the stack '%s'.\n"+
				"Check that all attributes in the stack name pattern '%s' are defined in stack config files.\n"+
				"Are the component and stack names correct? Did you forget an import?",
				component,
				stack,
				config.Config.Stacks.NamePattern,
			))
		}
	}

	baseComponentName := ""
	if baseComponent, baseComponentExist := componentSection["component"]; baseComponentExist {
		baseComponentName = baseComponent.(string)
	}

	// workspace
	var workspace string
	if len(baseComponentName) == 0 {
		workspace = stack
	} else {
		workspace = fmt.Sprintf("%s-%s", stack, component)
	}
	componentSection["workspace"] = strings.Replace(workspace, "/", "-", -1)

	return componentSection, nil
}

// ProcessComponentFromContext accepts context (tenant, environment, stage) and returns the component configuration in the stack
func ProcessComponentFromContext(component string, tenant string, environment string, stage string) (map[string]interface{}, error) {
	return ProcessComponentFromContextWithPath(component, tenant, environment, stage, "")
}

func ProcessComponentFromContextWithPath(component string, tenant string, environment string, stage string, basePath string) (map[string]interface{}, error) {
	var stack string

	err := config.InitConfig()
	if err != nil {
		return nil, err
	}

	if len(config.Config.Stacks.NamePattern) < 1 {
		return nil, errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
	}

	stackNamePatternParts := strings.Split(config.Config.Stacks.NamePattern, "-")

	for _, part := range stackNamePatternParts {
		if part == "{tenant}" {
			if len(tenant) == 0 {
				return nil, errors.New(fmt.Sprintf("stack name pattern '%s' includes '{tenant}', but tenant is not provided", config.Config.Stacks.NamePattern))
			}
			if len(stack) == 0 {
				stack = tenant
			} else {
				stack = fmt.Sprintf("%s-%s", stack, tenant)
			}
		} else if part == "{environment}" {
			if len(environment) == 0 {
				return nil, errors.New(fmt.Sprintf("stack name pattern '%s' includes '{environment}', but environment is not provided", config.Config.Stacks.NamePattern))
			}
			if len(stack) == 0 {
				stack = environment
			} else {
				stack = fmt.Sprintf("%s-%s", stack, environment)
			}
		} else if part == "{stage}" {
			if len(stage) == 0 {
				return nil, errors.New(fmt.Sprintf("stack name pattern '%s' includes '{stage}', but stage is not provided", config.Config.Stacks.NamePattern))
			}
			if len(stack) == 0 {
				stack = stage
			} else {
				stack = fmt.Sprintf("%s-%s", stack, stage)
			}
		}
	}

	return ProcessComponentInStackWithPath(component, stack, basePath)
}

// findComponentConfig finds component config sections
func findComponentConfig(
	stack string,
	stacksMap map[string]interface{},
	componentType string,
	component string,
) (map[string]interface{}, map[interface{}]interface{}, map[interface{}]interface{}, error) {

	var stackSection map[interface{}]interface{}
	var componentsSection map[string]interface{}
	var componentTypeSection map[string]interface{}
	var componentSection map[string]interface{}
	var componentVarsSection map[interface{}]interface{}
	var componentBackendSection map[interface{}]interface{}
	var ok bool

	if len(stack) == 0 {
		return nil, nil, nil, errors.New("Stack must be provided and must not be empty")
	}
	if len(component) == 0 {
		return nil, nil, nil, errors.New("Component must be provided and must not be empty")
	}
	if len(componentType) == 0 {
		return nil, nil, nil, errors.New("Component type must be provided and must not be empty")
	}
	if stackSection, ok = stacksMap[stack].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("Stack '%s' does not exist", stack))
	}
	if componentsSection, ok = stackSection["components"].(map[string]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("'components' section is missing in the stack '%s'", stack))
	}
	if componentTypeSection, ok = componentsSection[componentType].(map[string]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("'components/%s' section is missing in the stack '%s'", componentType, stack))
	}
	if componentSection, ok = componentTypeSection[component].(map[string]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("Invalid or missing configuration for the component '%s' in the stack '%s'", component, stack))
	}
	if componentVarsSection, ok = componentSection["vars"].(map[interface{}]interface{}); !ok {
		return nil, nil, nil, errors.New(fmt.Sprintf("Missing 'vars' section for the component '%s' in the stack '%s'", component, stack))
	}
	if componentBackendSection, ok = componentSection["backend"].(map[interface{}]interface{}); !ok {
		componentBackendSection = nil
	}

	return componentSection, componentVarsSection, componentBackendSection, nil
}
