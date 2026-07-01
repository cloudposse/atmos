package step

import (
	"context"
	"io"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ScriptHandler executes inline scripts with an explicit interpreter.
type ScriptHandler struct {
	BaseHandler
}

type scriptInvocation struct {
	interpreter string
	script      string
	workDir     string
}

func init() {
	Register(&ScriptHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeScript, CategoryCommand, false),
	})
}

// Validate checks that the script step has required fields and does not use command.
func (h *ScriptHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ScriptHandler.Validate")()

	if step.Command != "" {
		return errUtils.Build(schema.ErrScriptStepInvalidField).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			WithContext("field", "command").
			Err()
	}
	if err := h.ValidateRequired(step, "interpreter", step.Interpreter); err != nil {
		return err
	}
	return h.ValidateRequired(step, "script", step.Script)
}

// Execute runs the script.
func (h *ScriptHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ScriptHandler.Execute")()

	return h.execute(ctx, step, vars, nil)
}

// ExecuteWithWorkflow runs the script with workflow-level output defaults.
func (h *ScriptHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.ScriptHandler.ExecuteWithWorkflow")()

	return h.execute(ctx, step, vars, workflow)
}

func (h *ScriptHandler) execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	invocation, err := h.resolveInvocation(step, vars)
	if err != nil {
		return nil, err
	}
	env, err := h.resolveEnv(step, vars)
	if err != nil {
		return nil, err
	}

	mode := OutputMode(step.Output)
	if workflow != nil {
		mode = GetOutputMode(step, workflow)
	}
	if mode == "" {
		mode = OutputModeLog
	}

	writer := NewOutputModeWriter(mode, step.Name, GetViewportConfig(step, workflow), GetShowConfig(step, workflow))
	stdout, stderr, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		return process.RunScript(ctx, &process.ScriptSpec{
			Interpreter: invocation.interpreter,
			Script:      invocation.script,
			Name:        step.Name,
			Dir:         invocation.workDir,
			Env:         env,
			DryRun:      step.DryRun,
		}, stdout, stderr)
	})
	if err != nil {
		return NewStepResult(stdout).
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr).
			WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}

	return NewStepResult(stdout).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata(exitCodeMetadata, 0), nil
}

func (h *ScriptHandler) resolveInvocation(step *schema.WorkflowStep, vars *Variables) (scriptInvocation, error) {
	interpreter, err := vars.Resolve(step.Interpreter)
	if err != nil {
		return scriptInvocation{}, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "interpreter").
			Err()
	}
	script, err := vars.Resolve(step.Script)
	if err != nil {
		return scriptInvocation{}, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "script").
			Err()
	}
	workDir := step.WorkingDirectory
	if workDir != "" {
		workDir, err = vars.Resolve(workDir)
		if err != nil {
			return scriptInvocation{}, errUtils.Build(errUtils.ErrTemplateEvaluation).
				WithCause(err).
				WithContext("step", step.Name).
				WithContext("field", "working_directory").
				Err()
		}
	}
	return scriptInvocation{interpreter: interpreter, script: script, workDir: workDir}, nil
}

func (h *ScriptHandler) resolveEnv(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
	env := vars.EnvSlice()
	if len(env) == 0 {
		env = os.Environ()
	}
	if len(step.Env) == 0 {
		return env, nil
	}
	resolvedEnv, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrTemplateEvaluation).
			WithCause(err).
			WithContext("step", step.Name).
			WithContext("field", "env").
			Err()
	}
	for key, value := range resolvedEnv {
		env = envpkg.UpdateEnvVar(env, key, value)
	}
	return env, nil
}
