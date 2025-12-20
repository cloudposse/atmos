package step

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// AtmosHandler executes atmos commands.
type AtmosHandler struct {
	BaseHandler
}

func init() {
	Register(&AtmosHandler{
		BaseHandler: NewBaseHandler("atmos", CategoryCommand, false),
	})
}

// Validate checks that the step has required fields.
func (h *AtmosHandler) Validate(step *schema.WorkflowStep) error {
	return h.ValidateRequired(step, "command", step.Command)
}

// Execute runs the atmos command.
func (h *AtmosHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.AtmosHandler.Execute")()

	opts, err := h.prepareExecution(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	mode := OutputMode(step.Output)
	if mode == "" {
		mode = OutputModeLog
	}

	return h.runAtmosCommand(ctx, step.Name, opts, mode, step.Viewport)
}

// atmosExecOptions holds resolved options for command execution.
type atmosExecOptions struct {
	command string
	stack   string
	workDir string
	envVars []string
}

// prepareExecution resolves all step configuration for execution.
func (h *AtmosHandler) prepareExecution(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*atmosExecOptions, error) {
	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	stack, err := h.resolveStack(step, vars)
	if err != nil {
		return nil, err
	}

	workDir, err := h.resolveWorkDir(step, vars)
	if err != nil {
		return nil, err
	}

	envVars, err := h.resolveEnvVars(step, vars)
	if err != nil {
		return nil, err
	}

	return &atmosExecOptions{
		command: command,
		stack:   stack,
		workDir: workDir,
		envVars: envVars,
	}, nil
}

// resolveStack resolves the stack variable.
func (h *AtmosHandler) resolveStack(step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.Stack == "" {
		return "", nil
	}
	stack, err := vars.Resolve(step.Stack)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve stack: %w", step.Name, err)
	}
	return stack, nil
}

// resolveWorkDir resolves the working directory.
func (h *AtmosHandler) resolveWorkDir(step *schema.WorkflowStep, vars *Variables) (string, error) {
	if step.WorkingDirectory == "" {
		return "", nil
	}
	workDir, err := vars.Resolve(step.WorkingDirectory)
	if err != nil {
		return "", fmt.Errorf("step '%s': failed to resolve working_directory: %w", step.Name, err)
	}
	return workDir, nil
}

// resolveEnvVars resolves environment variables.
func (h *AtmosHandler) resolveEnvVars(step *schema.WorkflowStep, vars *Variables) ([]string, error) {
	if len(step.Env) == 0 {
		return nil, nil
	}
	resolvedEnv, err := vars.ResolveEnvMap(step.Env)
	if err != nil {
		return nil, fmt.Errorf("step '%s': %w", step.Name, err)
	}
	var envVars []string
	for k, v := range resolvedEnv {
		envVars = append(envVars, k+"="+v)
	}
	return envVars, nil
}

// runAtmosCommand executes the prepared atmos command.
func (h *AtmosHandler) runAtmosCommand(ctx context.Context, stepName string, opts *atmosExecOptions, mode OutputMode, viewport *schema.ViewportConfig) (*StepResult, error) {
	args := strings.Fields(opts.command)
	if opts.stack != "" && !containsStackFlag(args) {
		args = append(args, "-s", opts.stack)
	}

	// Use os.Executable() to get the absolute path to the currently running binary.
	// This ensures that the same binary is used even when invoked via relative paths,
	// symlinks, or from different working directories.
	atmosBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to determine atmos executable path: %w", err)
	}

	cmd := exec.CommandContext(ctx, atmosBin, args...)
	if opts.workDir != "" {
		cmd.Dir = opts.workDir
	}
	cmd.Env = append(os.Environ(), opts.envVars...)

	writer := NewOutputModeWriter(mode, stepName, viewport)
	stdout, stderr, err := writer.Execute(cmd)

	return h.buildAtmosResult(stdout, stderr, err), err
}

// buildAtmosResult creates a result from command output.
func (h *AtmosHandler) buildAtmosResult(stdout, stderr string, err error) *StepResult {
	if err != nil {
		return NewStepResult(stdout).
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr).
			WithMetadata("exit_code", getExitCode(err))
	}
	return NewStepResult(strings.TrimSpace(stdout)).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata("exit_code", 0)
}

// ExecuteWithWorkflow runs the atmos command with workflow context for output mode.
func (h *AtmosHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	opts, err := h.prepareExecution(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Get output mode from step or workflow.
	mode := GetOutputMode(step, workflow)
	viewport := GetViewportConfig(step, workflow)

	return h.runAtmosCommand(ctx, step.Name, opts, mode, viewport)
}

// containsStackFlag checks if args already contain -s or --stack.
func containsStackFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-s" || arg == "--stack" {
			return true
		}
		// Check for -s=value or --stack=value.
		if strings.HasPrefix(arg, "-s=") || strings.HasPrefix(arg, "--stack=") {
			return true
		}
	}
	return false
}
