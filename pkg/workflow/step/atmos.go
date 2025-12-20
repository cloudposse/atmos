package step

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve stack if specified.
	stack := step.Stack
	if stack != "" {
		stack, err = vars.Resolve(stack)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve stack: %w", step.Name, err)
		}
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

	// Build atmos command.
	args := strings.Fields(command)
	if stack != "" {
		// Add -s flag if stack is specified and not already in command.
		if !containsStackFlag(args) {
			args = append(args, "-s", stack)
		}
	}

	// Find atmos binary.
	atmosBin, err := exec.LookPath("atmos")
	if err != nil {
		atmosBin = "atmos" // Fall back to PATH lookup at runtime.
	}

	cmd := exec.CommandContext(ctx, atmosBin, args...) //nolint:gosec // Intentional command execution

	// Set working directory.
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set environment.
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
			WithMetadata("exit_code", getExitCode(err)), err
	}

	return NewStepResult(strings.TrimSpace(stdout)).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata("exit_code", 0), nil
}

// ExecuteWithWorkflow runs the atmos command with workflow context for output mode.
func (h *AtmosHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Resolve stack if specified.
	stack := step.Stack
	if stack != "" {
		stack, err = vars.Resolve(stack)
		if err != nil {
			return nil, fmt.Errorf("step '%s': failed to resolve stack: %w", step.Name, err)
		}
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

	// Build atmos command.
	args := strings.Fields(command)
	if stack != "" && !containsStackFlag(args) {
		args = append(args, "-s", stack)
	}

	// Find atmos binary.
	atmosBin, err := exec.LookPath("atmos")
	if err != nil {
		atmosBin = "atmos"
	}

	cmd := exec.CommandContext(ctx, atmosBin, args...) //nolint:gosec // Intentional command execution

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
			WithMetadata("exit_code", getExitCode(err)), err
	}

	return NewStepResult(strings.TrimSpace(stdout)).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata("exit_code", 0), nil
}

// containsStackFlag checks if args already contain -s or --stack.
func containsStackFlag(args []string) bool {
	for i, arg := range args {
		if arg == "-s" || arg == "--stack" {
			return true
		}
		// Check for -s=value or --stack=value.
		if strings.HasPrefix(arg, "-s=") || strings.HasPrefix(arg, "--stack=") {
			return true
		}
		// Check if next arg would be stack value for -s.
		if arg == "-s" && i+1 < len(args) {
			return true
		}
	}
	return false
}
