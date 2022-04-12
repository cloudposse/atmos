package component

import (
	"fmt"
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/pkg/errors"
	"strings"
)

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack
func ProcessComponentInStack(component string, stack string) (map[string]interface{}, error) {
	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err := e.ProcessStacks(configAndStacksInfo, true)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = e.ProcessStacks(configAndStacksInfo, true)
		if err != nil {
			return nil, err
		}
	}

	return configAndStacksInfo.ComponentSection, nil
}

// ProcessComponentFromContext accepts context (tenant, environment, stage) and returns the component configuration in the stack
func ProcessComponentFromContext(component string, tenant string, environment string, stage string) (map[string]interface{}, error) {
	var stack string

	err := c.InitConfig()
	if err != nil {
		return nil, err
	}

	if len(c.Config.Stacks.NamePattern) < 1 {
		return nil, errors.New("stack name pattern must be provided in 'stacks.name_pattern' config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
	}

	stackNamePatternParts := strings.Split(c.Config.Stacks.NamePattern, "-")

	for _, part := range stackNamePatternParts {
		if part == "{tenant}" {
			if len(tenant) == 0 {
				return nil, errors.New(fmt.Sprintf("stack name pattern '%s' includes '{tenant}', but tenant is not provided", c.Config.Stacks.NamePattern))
			}
			if len(stack) == 0 {
				stack = tenant
			} else {
				stack = fmt.Sprintf("%s-%s", stack, tenant)
			}
		} else if part == "{environment}" {
			if len(environment) == 0 {
				return nil, errors.New(fmt.Sprintf("stack name pattern '%s' includes '{environment}', but environment is not provided", c.Config.Stacks.NamePattern))
			}
			if len(stack) == 0 {
				stack = environment
			} else {
				stack = fmt.Sprintf("%s-%s", stack, environment)
			}
		} else if part == "{stage}" {
			if len(stage) == 0 {
				return nil, errors.New(fmt.Sprintf("stack name pattern '%s' includes '{stage}', but stage is not provided", c.Config.Stacks.NamePattern))
			}
			if len(stack) == 0 {
				stack = stage
			} else {
				stack = fmt.Sprintf("%s-%s", stack, stage)
			}
		}
	}

	return ProcessComponentInStack(component, stack)
}
