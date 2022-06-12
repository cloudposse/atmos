package component

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack
func ProcessComponentInStack(component string, stack string) (map[string]any, error) {
	var configAndStacksInfo c.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err := e.ProcessStacks(configAndStacksInfo, true)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = e.ProcessStacks(configAndStacksInfo, true)
		if err != nil {
			u.PrintErrorToStdError(err)
			return nil, err
		}
	}

	return configAndStacksInfo.ComponentSection, nil
}

// ProcessComponentFromContext accepts context (tenant, environment, stage) and returns the component configuration in the stack
func ProcessComponentFromContext(component string, tenant string, environment string, stage string) (map[string]any, error) {
	err := c.InitConfig()
	if err != nil {
		u.PrintErrorToStdError(err)
		return nil, err
	}

	if len(c.Config.Stacks.NamePattern) < 1 {
		er := errors.New("stack name pattern must be provided in 'stacks.name_pattern' CLI config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		u.PrintErrorToStdError(er)
		return nil, er
	}

	stack, err := c.GetStackNameFromContextAndStackNamePattern(tenant, environment, stage, c.Config.Stacks.NamePattern)
	if err != nil {
		u.PrintErrorToStdError(err)
		return nil, err
	}

	return ProcessComponentInStack(component, stack)
}
