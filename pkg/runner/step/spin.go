package step

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// SpinHandler displays a spinner while executing a command.
type SpinHandler struct {
	BaseHandler
}

func init() {
	Register(&SpinHandler{
		BaseHandler: NewBaseHandler("spin", CategoryOutput, false),
	})
}

// Validate checks that the step has required fields.
func (h *SpinHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.SpinHandler.Validate")()

	if err := h.ValidateRequired(step, "title", step.Title); err != nil {
		return err
	}
	return h.ValidateRequired(step, "command", step.Command)
}

// spinExecOptions holds resolved options for spin command execution.
type spinExecOptions struct {
	command string
	workDir string
	envVars []string
}

// Execute runs the command with a spinner.
func (h *SpinHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.SpinHandler.Execute")()

	title, err := vars.Resolve(step.Title)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
	}

	opts, err := h.prepareExecution(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	execCtx, cancel := h.createExecContext(ctx, step)
	if cancel != nil {
		defer cancel()
	}

	var stdout, stderr bytes.Buffer
	err = spinner.ExecWithSpinner(title, title, func() error {
		return h.runCommand(execCtx, opts, &stdout, &stderr)
	})

	return h.buildResult(stdout.String(), stderr.String(), err), err
}

// prepareExecution resolves all step configuration for execution.
func (h *SpinHandler) prepareExecution(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*spinExecOptions, error) {
	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	workDir := ""
	if step.WorkingDirectory != "" {
		workDir, err = vars.Resolve(step.WorkingDirectory)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve working_directory: %w", step.Name, err)
		}
	}

	// Build environment from Variables.Env (which includes prepared merged environment).
	// Guard against integer overflow when computing capacity.
	varsEnvLen := len(vars.Env)
	stepEnvLen := len(step.Env)
	capacity := varsEnvLen
	if varsEnvLen <= math.MaxInt-stepEnvLen {
		capacity = varsEnvLen + stepEnvLen
	}
	envVars := make([]string, 0, capacity)
	for k, v := range vars.Env {
		envVars = append(envVars, k+"="+v)
	}

	// Apply step-specific environment variable overrides.
	if len(step.Env) > 0 {
		resolvedEnv, err := vars.ResolveEnvMap(step.Env)
		if err != nil {
			return nil, fmt.Errorf("step '%s': %w", step.Name, err)
		}
		for k, v := range resolvedEnv {
			envVars = append(envVars, k+"="+v)
		}
	}

	return &spinExecOptions{
		command: command,
		workDir: workDir,
		envVars: envVars,
	}, nil
}

// createExecContext creates an execution context with optional timeout.
func (h *SpinHandler) createExecContext(ctx context.Context, step *schema.WorkflowStep) (context.Context, context.CancelFunc) {
	if step.Timeout == "" {
		return ctx, nil
	}

	timeout, err := time.ParseDuration(step.Timeout)
	if err != nil || timeout <= 0 {
		return ctx, nil
	}

	return context.WithTimeout(ctx, timeout)
}

// runCommand executes the command with configured options.
func (h *SpinHandler) runCommand(ctx context.Context, opts *spinExecOptions, stdout, stderr *bytes.Buffer) error {
	if opts.command == "" {
		return errUtils.ErrStepEmptyCommand
	}

	// Use shell to interpret the command string, supporting pipes, &&, etc.
	cmd := exec.CommandContext(ctx, "sh", "-c", opts.command) //nolint:gosec // User-provided command in workflow step
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if opts.workDir != "" {
		cmd.Dir = opts.workDir
	}

	// Use prepared environment from Variables.
	cmd.Env = opts.envVars

	return cmd.Run()
}

// buildResult creates a step result from command output.
func (h *SpinHandler) buildResult(stdout, stderr string, err error) *StepResult {
	if err != nil {
		return NewStepResult("").
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr)
	}

	return NewStepResult(stdout).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr)
}
