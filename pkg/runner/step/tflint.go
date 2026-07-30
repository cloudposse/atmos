package step

import (
	"context"
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners"
	tflintscanner "github.com/cloudposse/atmos/pkg/scanners/tflint"
	"github.com/cloudposse/atmos/pkg/schema"
)

const tflintStepType = "tflint"

var runTFLint = tflintscanner.Run

type TFLintHandler struct {
	BaseHandler
}

type tflintStepConfig struct {
	Component string            `yaml:"component"`
	Stack     string            `yaml:"stack"`
	Args      []string          `yaml:"args"`
	Env       map[string]string `yaml:"env"`
}

func init() {
	Register(&TFLintHandler{
		BaseHandler: NewBaseHandler(tflintStepType, CategoryCommand, false),
	})
}

func (h *TFLintHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.TFLintHandler.Validate")()

	stepCfg := tflintConfig(step)
	if strings.TrimSpace(stepCfg.Component) == "" {
		return errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("type", tflintStepType).
			WithContext("field", "component").
			WithExplanation("A tflint step must set `component` to the Terraform component name").
			Err()
	}
	return nil
}

func (h *TFLintHandler) Execute(ctx context.Context, workflowStep *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.TFLintHandler.Execute")()

	if err := h.Validate(workflowStep); err != nil {
		return nil, err
	}

	stepCfg := tflintConfig(workflowStep)

	component, err := vars.Resolve(stepCfg.Component)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve component: %w", workflowStep.Name, err)
	}
	stack, err := resolveTFLintStack(workflowStep, stepCfg, vars)
	if err != nil {
		return nil, err
	}
	args, err := resolveTFLintArgs(workflowStep, stepCfg, vars)
	if err != nil {
		return nil, err
	}
	env, err := vars.ResolveEnvMap(stepCfg.Env)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve env: %w", workflowStep.Name, err)
	}

	info, err := vars.ResolveComponentInfo(ctx, component, stack, cfg.TerraformComponentType)
	if err != nil {
		return nil, fmt.Errorf("step %q: failed to resolve component %q in stack %q: %w", workflowStep.Name, component, stack, err)
	}

	out, _, err := runTFLint(ctx, &tflintscanner.Options{
		Args:          args,
		Env:           env,
		BaseEnv:       vars.EnvSlice(),
		OnFailure:     scanners.OnFailureFail,
		AtmosConfig:   vars.AtmosConfig,
		Info:          info,
		ToolchainPATH: vars.ToolchainPATH,
	})

	result := tflintStepResult(component, stack, out)
	if err != nil {
		if result == nil {
			result = NewStepResult("")
		}
		result.WithError(err.Error())
		return result, err
	}
	return result, nil
}

func tflintConfig(step *schema.WorkflowStep) tflintStepConfig {
	if step == nil {
		return tflintStepConfig{}
	}
	cfg := decodeTFLintWith(step.With)
	if step.Component != "" {
		cfg.Component = step.Component
	}
	if step.Stack != "" {
		cfg.Stack = step.Stack
	}
	if len(step.Args) > 0 {
		cfg.Args = step.Args
	}
	if len(step.Env) > 0 {
		cfg.Env = step.Env
	}
	return cfg
}

func decodeTFLintWith(with map[string]any) tflintStepConfig {
	if len(with) == 0 {
		return tflintStepConfig{}
	}
	data, err := yaml.Marshal(with)
	if err != nil {
		return tflintStepConfig{}
	}
	var cfg tflintStepConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return tflintStepConfig{}
	}
	return cfg
}

func resolveTFLintStack(step *schema.WorkflowStep, cfg tflintStepConfig, vars *Variables) (string, error) {
	stack := strings.TrimSpace(cfg.Stack)
	if stack == "" {
		stack = vars.Flags["stack"]
	}
	if stack == "" {
		stack = vars.Env["ATMOS_STACK"]
	}
	if stack == "" {
		return "", errUtils.Build(errUtils.ErrStepFieldRequired).
			WithContext("step", step.Name).
			WithContext("type", tflintStepType).
			WithContext("field", "stack").
			WithExplanation("A tflint step must set `stack`, receive a workflow `--stack`, or run in a hook context with ATMOS_STACK").
			Err()
	}
	resolved, err := vars.Resolve(stack)
	if err != nil {
		return "", fmt.Errorf("step %q: failed to resolve stack: %w", step.Name, err)
	}
	return resolved, nil
}

func resolveTFLintArgs(step *schema.WorkflowStep, cfg tflintStepConfig, vars *Variables) ([]string, error) {
	args := tflintscanner.DefaultArgs()
	for _, arg := range cfg.Args {
		resolved, err := vars.Resolve(arg)
		if err != nil {
			return nil, fmt.Errorf("step %q: failed to resolve arg: %w", step.Name, err)
		}
		args = append(args, resolved)
	}
	return args, nil
}

func tflintStepResult(component, stack string, out *scanners.Output) *StepResult {
	result := NewStepResult("tflint completed").
		WithMetadata("component", component).
		WithMetadata("stack", stack)
	if out == nil || out.Summary == nil {
		return result
	}
	result.Value = out.Summary.Title
	result.WithMetadata("status", string(out.Summary.Status)).
		WithMetadata("title", out.Summary.Title).
		WithMetadata("counts", out.Summary.Counts).
		WithMetadata("findings", len(out.Summary.Findings))
	return result
}
