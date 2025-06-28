package component

import (
	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack.
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
		log.Error(err)
		return nil, err
	}

	configAndStacksInfo.ComponentType = "terraform"
	configAndStacksInfo, err = e.ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
	if err != nil {
		configAndStacksInfo.ComponentType = "helmfile"
		configAndStacksInfo, err = e.ProcessStacks(atmosConfig, configAndStacksInfo, true, true, true, nil)
		if err != nil {
			log.Error(err)
			return nil, err
		}
	}

	return configAndStacksInfo.ComponentSection, nil
}

// ProcessComponentFromContext accepts context (namespace, tenant, environment, stage) and returns the component configuration in the stack.
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
		log.Error(err)
		return nil, err
	}

	stackNameTemplate := e.GetStackNameTemplate(&atmosConfig)
	stackNamePattern := e.GetStackNamePattern(atmosConfig)
	var stack string

	switch {
	case stackNameTemplate != "":
		// Create the template context from the context variables.
		ctx := map[string]any{
			"vars": map[string]any{
				"namespace":   namespace,
				"tenant":      tenant,
				"environment": environment,
				"stage":       stage,
			},
		}

		stack, err = e.ProcessTmpl("name-template-from-context", stackNameTemplate, ctx, false)
		if err != nil {
			log.Error(err)
			return nil, err
		}

	case stackNamePattern != "":
		stack, err = cfg.GetStackNameFromContextAndStackNamePattern(namespace, tenant, environment, stage, stackNamePattern)
		if err != nil {
			log.Error(err)
			return nil, err
		}

	default:
		log.Error(errUtils.ErrMissingStackNameTemplateAndPattern)
		return nil, errUtils.ErrMissingStackNameTemplateAndPattern
	}

	return ProcessComponentInStack(component, stack, atmosCliConfigPath, atmosBasePath)
}
