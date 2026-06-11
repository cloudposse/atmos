package step

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
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
	defer perf.Track(nil, "step.ShellHandler.Validate")()

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

	// Terminal-attached or interactive steps need the session path for
	// platform-aware shell selection and direct terminal attachment.
	if step.Tty || step.Interactive {
		return h.executeShellSessionStep(ctx, step, command, workDir, envVars)
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

	mode, release := prepareInteractive(step, cmd, mode)
	defer release()

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
	var exitCodeErr errUtils.ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return exitCodeErr.Code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

// executeShellSessionStep runs a terminal-attached or interactive step.
// Session steps produce no capturable output, so the StepResult carries an
// empty output and only the exit code.
func (h *ShellHandler) executeShellSessionStep(ctx context.Context, step *schema.WorkflowStep, command, workDir string, envVars []string) (*StepResult, error) {
	if step.Output != "" {
		log.Debug("Output mode ignored for shell session step", "step", step.Name, "output", step.Output)
	}

	err := process.RunShellSession(ctx, &process.ShellSessionSpec{
		Command:       command,
		Name:          step.Name,
		Dir:           workDir,
		Env:           append(os.Environ(), envVars...),
		TTY:           step.Tty,
		Interactive:   step.Interactive,
		Masker:        iolib.GetContext().Masker(),
		EnableMasking: viper.GetBool("mask"),
	})
	if err != nil {
		return NewStepResult("").WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}
	return NewStepResult("").WithMetadata(exitCodeMetadata, 0), nil
}

// prepareInteractive adapts command execution for `interactive: true` steps:
// host stdin is attached, buffered output modes are forced to raw (they would
// hide the prompts the user must respond to), and the Atmos SIGINT-exit
// handler is suspended so the step owns Ctrl-C. The returned release function
// must be called after execution; it is a no-op for non-interactive steps.
func prepareInteractive(step *schema.WorkflowStep, cmd *exec.Cmd, mode OutputMode) (OutputMode, func()) {
	if !step.Interactive {
		return mode, func() {}
	}

	if mode != OutputModeRaw {
		log.Debug("Forcing raw output mode for interactive step", "step", step.Name, "output", mode)
		mode = OutputModeRaw
	}
	cmd.Stdin = os.Stdin
	return mode, signals.SuspendInterruptExit()
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

	// Terminal-attached or interactive steps need the session path for
	// platform-aware shell selection and direct terminal attachment.
	if step.Tty || step.Interactive {
		return h.executeShellSessionStep(ctx, step, command, workDir, envVars)
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

	mode, release := prepareInteractive(step, cmd, mode)
	defer release()

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
