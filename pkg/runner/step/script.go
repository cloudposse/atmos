package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	osexec "os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	_ "github.com/cloudposse/atmos/pkg/script/starlark" // Register the embedded interpreter.
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

	if err := h.ValidateOutput(step); err != nil {
		return err
	}
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
	if _, embedded := script.Get(step.Interpreter); embedded && step.Container.IsEnabled() {
		return fmt.Errorf("%w: embedded %s requires container: false", errUtils.ErrScript, strings.TrimSpace(step.Interpreter))
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
	// The raw interpreter may be a template, so Validate cannot tell whether it is embedded.
	// Check the rendered interpreter here, where the container opt-in is still visible.
	if _, embedded := script.Get(invocation.interpreter); embedded && step.Container.IsEnabled() {
		return nil, fmt.Errorf("%w: embedded %s requires container: false", errUtils.ErrScript, strings.TrimSpace(invocation.interpreter))
	}
	env, err := h.resolveEnv(step, vars)
	if err != nil {
		return nil, err
	}

	// Render a templated `output:` before anything runs, so an unknown mode fails the step.
	step, err = resolveOutputStep(step, vars)
	if err != nil {
		return nil, err
	}

	// Enforce the step's timeout by running the interpreter under a context deadline. The embedded
	// engine cancels its thread and subprocesses when the context ends.
	deadline, err := StartStepDeadline(ctx, step, vars)
	if err != nil {
		return nil, err
	}
	defer deadline.Stop()
	ctx = deadline.Context()

	// Script steps share the command-step output defaults: raw mode and no step labels unless
	// the step or workflow opts in through output or show.labels.
	writer := NewCommandOutputWriter(step, workflow)
	writer.writers = vars.OutputWriters
	var embedded script.Result
	stdout, stderr, err := writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		if engine, ok := script.Get(invocation.interpreter); ok {
			inputs := step.Env
			if step.ScriptEnv != nil {
				inputs = step.ScriptEnv
			}
			resolved, resolveErr := vars.ResolveStepEnvMap(step, inputs)
			if resolveErr != nil {
				return resolveErr
			}
			var runErr error
			embedded, runErr = engine.Execute(ctx, script.Spec{
				Steps:        NewAutomationLibrary(vars, workflow),
				Parallel:     vars.automationParallel,
				InstallTools: ScriptToolInstaller(vars.AtmosConfig),
				Name:         step.Name, Source: invocation.script, WorkingDirectory: invocation.workDir,
				SourcePath: step.ScriptSource, ProjectRoot: scriptProjectRoot(vars),
				Env: resolved, ProcessEnv: env, Stdout: stdout, Stderr: stderr, DryRun: step.DryRun,
				Flags: vars.ScriptFlags(), Arguments: vars.ScriptArguments(),
				Component: ScriptComponentRef(vars), ResolveComponent: ScriptComponentResolver(vars),
				ProcessOverrides: step.ScriptProcessOverrides,
				Hook:             vars.ScriptHook,
				Args:             vars.ScriptArgs,
			})
			return runErr
		}
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
		err = deadline.Wrap(err)
		err = WrapScriptInterpreterError(invocation.interpreter, err)
		return NewStepResult(stdout).
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr).
			WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}

	value := stdout
	if embedded.HasOutput {
		value = embedded.Value
	}
	return NewStepResult(value).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata(exitCodeMetadata, 0), nil
}

// WrapScriptInterpreterError adds a naming hint when an external interpreter could not be
// started because its name differs only by case from a registered embedded interpreter
// (for example `Starlark`). It does nothing on the success path and for any other failure,
// so the lookup runs only after an interpreter was already not found.
func WrapScriptInterpreterError(interpreter string, err error) error {
	defer perf.Track(nil, "step.WrapScriptInterpreterError")()

	if err == nil || !errors.Is(err, osexec.ErrNotFound) {
		return err
	}
	name := strings.TrimSpace(interpreter)
	lower := strings.ToLower(name)
	if lower == name {
		return err
	}
	if _, embedded := script.Get(lower); !embedded {
		return err
	}
	return errUtils.Build(err).
		WithHintf("Did you mean `%s`? Embedded interpreter names are case-sensitive.", lower).
		Err()
}

func (h *ScriptHandler) resolveInvocation(step *schema.WorkflowStep, vars *Variables) (scriptInvocation, error) {
	interpreter, err := vars.ResolveStepField(step, "interpreter", step.Interpreter)
	if err != nil {
		return scriptInvocation{}, TemplateFieldError(step, "interpreter", err)
	}
	script, err := vars.ResolveStepField(step, "script", step.Script)
	if err != nil {
		return scriptInvocation{}, TemplateFieldError(step, "script", err)
	}
	workDir := step.WorkingDirectory
	if workDir != "" {
		workDir, err = vars.ResolveStepField(step, "working_directory", workDir)
		if err != nil {
			return scriptInvocation{}, TemplateFieldError(step, "working_directory", err)
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
	resolvedEnv, err := vars.ResolveStepEnvMap(step, step.Env)
	if err != nil {
		return nil, TemplateFieldError(step, "env", err)
	}
	for key, value := range resolvedEnv {
		env = envpkg.UpdateEnvVar(env, key, value)
	}
	return env, nil
}

// ScriptFlags snapshots parsed flags without rendering user-provided strings.
// Custom commands preserve native types in their template roots; workflows use
// the string-valued flag map. Neither path needs to materialize prior step output.
func (v *Variables) ScriptFlags() map[string]any {
	defer perf.Track(nil, "step.Variables.ScriptFlags")()

	if v == nil {
		return nil
	}

	for _, key := range []string{"Flags", "flags"} {
		if value, exists := v.templateRoots[key]; exists {
			return scriptInputMap(value)
		}
	}
	return scriptInputMap(v.Flags)
}

// ScriptArguments snapshots named command arguments for embedded scripts.
func (v *Variables) ScriptArguments() map[string]any {
	defer perf.Track(nil, "step.Variables.ScriptArguments")()

	if v == nil {
		return nil
	}

	return scriptInputMap(v.templateRoots["Arguments"])
}

func scriptInputMap(value any) map[string]any {
	defer perf.Track(nil, "step.scriptInputMap")()

	switch input := value.(type) {
	case map[string]any:
		return maps.Clone(input)
	case map[string]string:
		result := make(map[string]any, len(input))
		for key, item := range input {
			result[key] = item
		}
		return result
	default:
		return nil
	}
}
