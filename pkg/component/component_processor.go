package component

import (
	"github.com/pkg/errors"
	"path/filepath"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack
func ProcessComponentInStack(
	component string,
	stack string,
	atmosCliConfigPath string,
	atmosBasePath string,
) (map[string]any, error) {

	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack
	configAndStacksInfo.AtmosCliConfigPath = atmosCliConfigPath
	configAndStacksInfo.AtmosBasePath = atmosBasePath

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		u.LogError(atmosConfig, err)
		return nil, err
	}

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err = e.ProcessStacks(atmosConfig, configAndStacksInfo, true, true)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = e.ProcessStacks(atmosConfig, configAndStacksInfo, true, true)
		if err != nil {
			u.LogError(atmosConfig, err)
			return nil, err
		}
	}

	// Convert any absolute paths in deps to relative paths
	if deps, ok := configAndStacksInfo.ComponentSection["deps"].([]any); ok {
		for i, dep := range deps {
			if depStr, ok := dep.(string); ok {
				// Convert absolute path to relative path if it starts with the base path
				if atmosBasePath != "" && strings.HasPrefix(depStr, atmosBasePath) {
					relPath, err := filepath.Rel(atmosBasePath, depStr)
					if err == nil {
						deps[i] = relPath
					}
				}
			}
		}
		configAndStacksInfo.ComponentSection["deps"] = deps
	}

	return configAndStacksInfo.ComponentSection, nil
}

// ProcessComponentFromContext accepts context (namespace, tenant, environment, stage) and returns the component configuration in the stack
func ProcessComponentFromContext(
	component string,
	namespace string,
	tenant string,
	environment string,
	stage string,
	atmosCliConfigPath string,
	atmosBasePath string,
) (map[string]any, error) {

	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.AtmosCliConfigPath = atmosCliConfigPath
	configAndStacksInfo.AtmosBasePath = atmosBasePath

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		u.LogError(atmosConfig, err)
		return nil, err
	}

	if len(e.GetStackNamePattern(atmosConfig)) < 1 {
		er := errors.New("stack name pattern must be provided in 'stacks.name_pattern' CLI config or 'ATMOS_STACKS_NAME_PATTERN' ENV variable")
		u.LogError(atmosConfig, er)
		return nil, er
	}

	stack, err := cfg.GetStackNameFromContextAndStackNamePattern(namespace, tenant, environment, stage, e.GetStackNamePattern(atmosConfig))
	if err != nil {
		u.LogError(atmosConfig, err)
		return nil, err
	}

	return ProcessComponentInStack(component, stack, atmosCliConfigPath, atmosBasePath)
}
