package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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

	envVars, err := h.resolveEnv(step, vars)
	if err != nil {
		return nil, err
	}

	// Terminal-attached or interactive steps need the session path for
	// platform-aware shell selection and direct terminal attachment.
	if step.Tty || step.Interactive {
		return h.executeShellSessionStep(ctx, step, command, workDir, envVars)
	}

	// Get output mode - use default log mode if not in workflow context.
	mode := OutputMode(step.Output)
	if mode == "" {
		mode = OutputModeLog
	}

	writer := NewOutputModeWriter(mode, step.Name, step.Viewport, GetShowConfig(step, nil))
	stdout, stderr, err := h.runInterpreter(ctx, writer, shellRunSpec{stepName: step.Name, command: command, workDir: workDir, env: envVars})
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

// shellRunSpec holds the resolved inputs for a single shell execution.
type shellRunSpec struct {
	stepName string
	command  string
	workDir  string
	env      []string
}

// runInterpreter executes a shell command through the in-process mvdan/sh
// interpreter with secret masking, replacing a host `sh -c` subprocess. This
// gives registry consumers (hooks, custom commands, parallel/matrix children)
// the same cross-platform, masked, exit-code-preserving shell semantics the
// workflow executor already used, instead of depending on the host `/bin/sh`.
func (h *ShellHandler) runInterpreter(ctx context.Context, writer *OutputModeWriter, spec shellRunSpec) (string, string, error) {
	return writer.ExecuteWithIO(func(stdout, stderr io.Writer) error {
		return u.ShellRunnerWithWriters(&u.ShellRunnerSpec{
			Context: ctx,
			Command: spec.command,
			Name:    spec.stepName,
			Dir:     spec.workDir,
			Env:     spec.env,
			Stdout:  iolib.MaskWriter(stdout),
			Stderr:  iolib.MaskWriter(stderr),
		})
	})
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
		Command:     command,
		Name:        step.Name,
		Dir:         workDir,
		Env:         envVars,
		TTY:         step.Tty,
		Interactive: step.Interactive,
	})
	if err != nil {
		return NewStepResult("").WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}
	return NewStepResult("").WithMetadata(exitCodeMetadata, 0), nil
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

	envVars, err := h.resolveEnv(step, vars)
	if err != nil {
		return nil, err
	}

	// Terminal-attached or interactive steps need the session path for
	// platform-aware shell selection and direct terminal attachment.
	if step.Tty || step.Interactive {
		return h.executeShellSessionStep(ctx, step, command, workDir, envVars)
	}

	// Get output mode from step or workflow.
	mode := GetOutputMode(step, workflow)
	viewport := GetViewportConfig(step, workflow)
	show := GetShowConfig(step, workflow)

	writer := NewOutputModeWriter(mode, step.Name, viewport, show)
	stdout, stderr, err := h.runInterpreter(ctx, writer, shellRunSpec{stepName: step.Name, command: command, workDir: workDir, env: envVars})
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

func (h *ShellHandler) resolveEnv(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
	env := vars.EnvSlice()
	if len(env) == 0 {
		env = os.Environ()
	}
	if len(step.Env) == 0 {
		return env, nil
	}
	resolvedEnv, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}
	for key, value := range resolvedEnv {
		env = envpkg.UpdateEnvVar(env, key, value)
	}
	return env, nil
}
