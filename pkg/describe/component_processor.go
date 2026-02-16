package describe

import (
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentFromContextParams contains the parameters for ProcessComponentFromContext.
type ComponentFromContextParams struct {
	Component          string
	Namespace          string
	Tenant             string
	Environment        string
	Stage              string
	AtmosCliConfigPath string
	AtmosBasePath      string
}

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack.
//
// This function is the public API used by terraform-provider-utils (data "utils_component_config").
// It was originally in pkg/component/component_processor.go but was moved here because
// internal/exec imports pkg/component (for the registry/provider types), creating an import
// cycle if pkg/component imports internal/exec.
func ProcessComponentInStack(
	component string,
	stack string,
	atmosCliConfigPath string,
	atmosBasePath string,
) (map[string]any, error) {
	defer perf.Track(nil, "describe.ProcessComponentInStack")()

	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = component
	configAndStacksInfo.Stack = stack
	configAndStacksInfo.AtmosCliConfigPath = atmosCliConfigPath
	configAndStacksInfo.AtmosBasePath = atmosBasePath

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
}

// ProcessComponentFromContext accepts context (namespace, tenant, environment, stage)
// and returns the component configuration in the stack.
//
// This function is the public API used by terraform-provider-utils (data "utils_component_config").
// See ProcessComponentInStack for details on why this lives in pkg/describe.
func ProcessComponentFromContext(params *ComponentFromContextParams) (map[string]any, error) {
	defer perf.Track(nil, "describe.ProcessComponentFromContext")()

	var configAndStacksInfo schema.ConfigAndStacksInfo
	configAndStacksInfo.ComponentFromArg = params.Component
	configAndStacksInfo.AtmosCliConfigPath = params.AtmosCliConfigPath
	configAndStacksInfo.AtmosBasePath = params.AtmosBasePath

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	stackNameTemplate := e.GetStackNameTemplate(&atmosConfig)
	stackNamePattern := e.GetStackNamePattern(&atmosConfig)
	var stack string

	switch {
	case stackNameTemplate != "":
		ctx := map[string]any{
			"vars": map[string]any{
				"namespace":   params.Namespace,
				"tenant":      params.Tenant,
				"environment": params.Environment,
				"stage":       params.Stage,
			},
		}
		stack, err = e.ProcessTmpl(&atmosConfig, "name-template-from-context", stackNameTemplate, ctx, false)
		if err != nil {
			return nil, err
		}

	case stackNamePattern != "":
		stack, err = cfg.GetStackNameFromContextAndStackNamePattern(params.Namespace, params.Tenant, params.Environment, params.Stage, stackNamePattern)
		if err != nil {
			return nil, err
		}

	default:
		return nil, errUtils.ErrMissingStackNameTemplateAndPattern
	}

	return ProcessComponentInStack(params.Component, stack, params.AtmosCliConfigPath, params.AtmosBasePath)
}
