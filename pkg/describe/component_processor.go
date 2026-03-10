package describe

import (
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ProcessComponentInStackOptions provides optional processing controls for ProcessComponentInStack.
type ProcessComponentInStackOptions struct {
	ProcessTemplates     *bool // Controls Go template resolution. Defaults to true if nil.
	ProcessYamlFunctions *bool // Controls YAML function resolution (!terraform.output, etc). Defaults to true if nil.
}

// ComponentFromContextParams contains the parameters for ProcessComponentFromContext.
type ComponentFromContextParams struct {
	Component            string
	Namespace            string
	Tenant               string
	Environment          string
	Stage                string
	AtmosCliConfigPath   string
	AtmosBasePath        string
	ProcessTemplates     *bool // Optional: controls Go template resolution. Defaults to true if nil.
	ProcessYamlFunctions *bool // Optional: controls YAML function resolution (!terraform.output, etc). Defaults to true if nil.
}

// ProcessComponentInStack accepts a component and a stack name and returns the component configuration in the stack.
//
// This function is the public API used by terraform-provider-utils (data "utils_component_config").
// It was originally in pkg/component/component_processor.go but was moved here because
// internal/exec imports pkg/component (for the registry/provider types), creating an import
// cycle if pkg/component imports internal/exec.
//
// An optional ProcessComponentInStackOptions struct can be passed to control whether Go templates
// and YAML functions (e.g. !terraform.output, !terraform.state) are resolved during processing.
// When called from the terraform-provider-utils, both should be false to avoid spawning child
// terraform processes inside the provider plugin. When omitted, both default to true for
// backward compatibility.
func ProcessComponentInStack(
	component string,
	stack string,
	atmosCliConfigPath string,
	atmosBasePath string,
	opts ...ProcessComponentInStackOptions,
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

	var processTemplates, processYamlFunctions *bool
	if len(opts) > 0 {
		processTemplates = opts[0].ProcessTemplates
		processYamlFunctions = opts[0].ProcessYamlFunctions
	}

	return processComponentInStackWithConfig(&atmosConfig, component, stack, processTemplates, processYamlFunctions)
}

// ProcessComponentFromContext accepts context (namespace, tenant, environment, stage)
// and returns the component configuration in the stack.
//
// This function is the public API used by terraform-provider-utils (data "utils_component_config").
// See ProcessComponentInStack for details on why this lives in pkg/describe.
func ProcessComponentFromContext(params *ComponentFromContextParams) (map[string]any, error) {
	defer perf.Track(nil, "describe.ProcessComponentFromContext")()

	if params == nil {
		return nil, errUtils.ErrNilParam
	}

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

	return processComponentInStackWithConfig(&atmosConfig, params.Component, stack, params.ProcessTemplates, params.ProcessYamlFunctions)
}

// boolDefault returns the value pointed to by p, or defaultVal if p is nil.
func boolDefault(p *bool, defaultVal bool) bool {
	if p != nil {
		return *p
	}
	return defaultVal
}

// processComponentInStackWithConfig is the shared implementation used by both public functions.
// It accepts an already-initialized AtmosConfiguration to avoid redundant config parsing.
// ProcessTemplates and ProcessYamlFunctions control resolution; nil defaults to true.
func processComponentInStackWithConfig(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	processTemplates *bool,
	processYamlFunctions *bool,
) (map[string]any, error) {
	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     boolDefault(processTemplates, true),
		ProcessYamlFunctions: boolDefault(processYamlFunctions, true),
	})
}
