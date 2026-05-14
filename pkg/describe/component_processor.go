package describe

import (
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// processOptions holds the resolved processing options for component resolution.
type processOptions struct {
	processTemplates     bool
	processYamlFunctions bool
}

// ProcessOption is a functional option for ProcessComponentInStack and ProcessComponentFromContext.
type ProcessOption func(*processOptions)

// WithProcessTemplates controls whether Go templates are resolved during processing.
// When false, template expressions like {{ .settings.config.a }} are preserved as raw strings.
// Defaults to true when not specified.
//
//nolint:lintroller // Trivial closure constructor - no perf tracking needed.
func WithProcessTemplates(enabled bool) ProcessOption {
	return func(o *processOptions) {
		o.processTemplates = enabled
	}
}

// WithProcessYamlFunctions controls whether YAML functions (!terraform.output, !terraform.state,
// !template, !store, etc.) are resolved during processing.
// When false, YAML function tags are preserved as raw strings.
// Defaults to true when not specified.
//
//nolint:lintroller // Trivial closure constructor - no perf tracking needed.
func WithProcessYamlFunctions(enabled bool) ProcessOption {
	return func(o *processOptions) {
		o.processYamlFunctions = enabled
	}
}

// defaultProcessOptions returns the default processing options (both enabled).
func defaultProcessOptions() processOptions {
	return processOptions{
		processTemplates:     true,
		processYamlFunctions: true,
	}
}

// applyProcessOptions applies functional options to the default options.
func applyProcessOptions(opts []ProcessOption) processOptions {
	o := defaultProcessOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

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
//
// Functional options can be passed to control whether Go templates and YAML functions
// (e.g. !terraform.output, !terraform.state) are resolved during processing.
// When called from the terraform-provider-utils, both should be disabled to avoid spawning
// child terraform processes inside the provider plugin. When omitted, both default to true
// for backward compatibility.
func ProcessComponentInStack(
	component string,
	stack string,
	atmosCliConfigPath string,
	atmosBasePath string,
	opts ...ProcessOption,
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

	o := applyProcessOptions(opts)

	return processComponentInStackWithConfig(&atmosConfig, component, stack, &o)
}

// ProcessComponentFromContext accepts context (namespace, tenant, environment, stage)
// and returns the component configuration in the stack.
//
// This function is the public API used by terraform-provider-utils (data "utils_component_config").
// See ProcessComponentInStack for details on why this lives in pkg/describe.
func ProcessComponentFromContext(params *ComponentFromContextParams, opts ...ProcessOption) (map[string]any, error) {
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

	o := applyProcessOptions(opts)

	return processComponentInStackWithConfig(&atmosConfig, params.Component, stack, &o)
}

// processComponentInStackWithConfig is the shared implementation used by both public functions.
// It accepts an already-initialized AtmosConfiguration to avoid redundant config parsing.
func processComponentInStackWithConfig(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	opts *processOptions,
) (map[string]any, error) {
	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     opts.processTemplates,
		ProcessYamlFunctions: opts.processYamlFunctions,
	})
}
