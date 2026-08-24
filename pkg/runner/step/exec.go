package step

import (
	"context"
	"fmt"

	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecHandler runs a command that replaces the Atmos process (exec semantics),
// so `type: exec` is a recognized, executable step type through the registry —
// previously it existed only in the legacy workflow executor.
type ExecHandler struct {
	BaseHandler
}

func init() {
	Register(&ExecHandler{
		BaseHandler: NewBaseHandler(schema.TaskTypeExec, CategoryCommand, false),
	})
}

// Validate checks that the exec step has a command. The "must be the final step"
// constraint is enforced at the workflow level (schema.ValidateWorkflowSteps); a
// standalone registry dispatch (hook, custom command) is inherently final.
func (h *ExecHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ExecHandler.Validate")()

	return h.ValidateRequired(step, "command", step.Command)
}

// Execute replaces the current process with the command (exec syscall on Unix).
// On success under Unix this call never returns; on Windows the command runs to
// completion and its error (if any) is returned. No retry wrapper is applied: a
// replaced process can never return to retry.
func (h *ExecHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ExecHandler.Execute")()

	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	workDir := step.WorkingDirectory
	if workDir != "" {
		workDir, err = vars.Resolve(workDir)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve working_directory: %w", step.Name, err)
		}
	}

	env, err := h.execEnv(step, vars)
	if err != nil {
		return nil, err
	}

	if err := process.ReplaceShellSession(&process.ExecSpec{
		Command: command,
		Name:    step.Name,
		Dir:     workDir,
		Env:     env,
	}); err != nil {
		return NewStepResult("").WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}
	return NewStepResult("").WithMetadata(exitCodeMetadata, 0), nil
}

// execEnv builds the fully merged environment for the replacement process.
func (h *ExecHandler) execEnv(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
	env := vars.EnvSlice()
	if len(step.Env) == 0 {
		return env, nil
	}
	resolved, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}
	for k, v := range resolved {
		env = envpkg.UpdateEnvVar(env, k, v)
	}
	return env, nil
}
