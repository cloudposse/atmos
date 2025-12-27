package step

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const exitCodeMetadata = "exit_code"

// ShellHandler executes shell commands.
type ShellHandler struct {
	BaseHandler
}

func init() {
	Register(&ShellHandler{
		BaseHandler: NewBaseHandler("shell", CategoryCommand, false),
	})
}

// Validate checks that the step has required fields.
func (h *ShellHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "command", step.Command)
}

// Execute runs the shell command.
func (h *ShellHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ShellHandler.Execute")()

	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve working directory if specified.
	workDir := step.WorkingDirectory
	if workDir != "" {
		workDir, err = vars.Resolve(workDir)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve working_directory: %w", step.Name, err)
		}
	}

	// Resolve environment variables.
	var envVars []string
	if len(step.Env) > 0 {
		resolvedEnv, err := vars.ResolveEnvMap(step.Env)
		if err != nil {
			return nil, fmt.Errorf("step '%s': %w", step.Name, err)
		}
		for k, v := range resolvedEnv {
			envVars = append(envVars, k+"="+v)
		}
	}

	// Create command - use shell to interpret the command string.
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	// Set working directory.
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set environment - inherit current environment and add custom vars.
	cmd.Env = append(os.Environ(), envVars...)

	// Get output mode - use default log mode if not in workflow context.
	mode := OutputMode(step.Output)
	if mode == "" {
		mode = OutputModeLog
	}

	// Execute with output mode handling.
	writer := NewOutputModeWriter(mode, step.Name, step.Viewport)
	stdout, stderr, err := writer.Execute(cmd)
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

// getExitCode extracts exit code from error.
func getExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// ExecuteWithWorkflow runs the shell command with workflow context for output mode.
func (h *ShellHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.ShellHandler.ExecuteWithWorkflow")()

	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve working directory if specified.
	workDir := step.WorkingDirectory
	if workDir != "" {
		workDir, err = vars.Resolve(workDir)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve working_directory: %w", step.Name, err)
		}
	}

	// Resolve environment variables.
	var envVars []string
	if len(step.Env) > 0 {
		resolvedEnv, err := vars.ResolveEnvMap(step.Env)
		if err != nil {
			return nil, fmt.Errorf("step '%s': %w", step.Name, err)
		}
		for k, v := range resolvedEnv {
			envVars = append(envVars, k+"="+v)
		}
	}

	// Create command.
	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	// Set working directory.
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set environment.
	cmd.Env = append(os.Environ(), envVars...)

	// Get output mode from step or workflow.
	mode := GetOutputMode(step, workflow)
	viewport := GetViewportConfig(step, workflow)

	// Execute with output mode handling.
	writer := NewOutputModeWriter(mode, step.Name, viewport)
	stdout, stderr, err := writer.Execute(cmd)
	if err != nil {
		return NewStepResult(stdout).
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr).
			WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}

	return NewStepResult(strings.TrimSpace(stdout)).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata(exitCodeMetadata, 0), nil
}
