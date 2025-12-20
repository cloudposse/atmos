package step

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

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
	if err := h.ValidateRequired(step, "title", step.Title); err != nil {
		return err
	}
	return h.ValidateRequired(step, "command", step.Command)
}

// Execute runs the command with a spinner.
func (h *SpinHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	title, err := vars.Resolve(step.Title)
	if err != nil {
		return nil, fmt.Errorf("step '%s': failed to resolve title: %w", step.Name, err)
	}

	command, err := h.ResolveCommand(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Parse timeout if specified.
	var timeout time.Duration
	if step.Timeout != "" {
		timeout, err = time.ParseDuration(step.Timeout)
		if err != nil {
			return nil, fmt.Errorf("step '%s': invalid timeout: %w", step.Name, err)
		}
	}

	// Create context with timeout if specified.
	execCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Capture command output.
	var stdout, stderr bytes.Buffer

	// Run command with spinner.
	err = spinner.ExecWithSpinner(title, title, func() error {
		// Parse command - simple split on spaces (TODO: use shlex for proper parsing).
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return fmt.Errorf("empty command")
		}

		cmd := exec.CommandContext(execCtx, parts[0], parts[1:]...) //nolint:gosec // Intentional command execution
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Set working directory if specified.
		if step.WorkingDirectory != "" {
			workDir, err := vars.Resolve(step.WorkingDirectory)
			if err != nil {
				return fmt.Errorf("failed to resolve working_directory: %w", err)
			}
			cmd.Dir = workDir
		}

		// Resolve and set environment variables.
		if len(step.Env) > 0 {
			resolvedEnv, err := vars.ResolveEnvMap(step.Env)
			if err != nil {
				return err
			}
			for k, v := range resolvedEnv {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}

		return cmd.Run()
	})
	if err != nil {
		return NewStepResult("").
			WithError(stderr.String()).
			WithMetadata("stdout", stdout.String()).
			WithMetadata("stderr", stderr.String()), err
	}

	return NewStepResult(stdout.String()).
		WithMetadata("stdout", stdout.String()).
		WithMetadata("stderr", stderr.String()), nil
}
