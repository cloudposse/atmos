package step

import (
	"context"
	"fmt"
	"sort"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
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
	defer perf.Track(nil, "step.ShellHandler.Validate")()

	return h.ValidateRequired(step, "command", step.Command)
}

type shellExecutionConfig struct {
	command string
	workDir string
	env     []string
}

type shellExecutionRequest struct {
	step      *schema.WorkflowStep
	config    *shellExecutionConfig
	mode      OutputMode
	viewport  *schema.ViewportConfig
	trimValue bool
}

// Execute runs the shell command.
func (h *ShellHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.ShellHandler.Execute")()

	cfg, err := h.prepareExecution(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Get output mode - use default log mode if not in workflow context.
	mode := OutputMode(step.Output)
	if mode == "" {
		mode = OutputModeLog
	}

	return h.executePrepared(ctx, shellExecutionRequest{
		step:     step,
		config:   cfg,
		mode:     mode,
		viewport: step.Viewport,
	})
}

// getExitCode extracts exit code from error.
func getExitCode(err error) int {
	return process.ExitCode(err)
}

// executeShellSessionStep runs a terminal-attached or interactive step.
// Session steps produce no capturable output, so the StepResult carries an
// empty output and only the exit code.
func (h *ShellHandler) executeShellSessionStep(ctx context.Context, step *schema.WorkflowStep, cfg *shellExecutionConfig) (*StepResult, error) {
	if step.Output != "" {
		log.Debug("Output mode ignored for shell session step", "step", step.Name, "output", step.Output)
	}

	err := process.RunShellSession(ctx, &process.ShellSessionSpec{
		Command:     cfg.command,
		Name:        step.Name,
		Dir:         cfg.workDir,
		Env:         cfg.env,
		TTY:         step.Tty,
		Interactive: step.Interactive,
		DryRun:      executionOptionsFromContext(ctx).DryRun,
	})
	if err != nil {
		return NewStepResult("").WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}
	return NewStepResult("").WithMetadata(exitCodeMetadata, 0), nil
}

// ExecuteWithWorkflow runs the shell command with workflow context for output mode.
func (h *ShellHandler) ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error) {
	defer perf.Track(nil, "step.ShellHandler.ExecuteWithWorkflow")()

	cfg, err := h.prepareExecution(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	// Get output mode from step or workflow.
	mode := GetOutputMode(step, workflow)
	viewport := GetViewportConfig(step, workflow)

	return h.executePrepared(ctx, shellExecutionRequest{
		step:      step,
		config:    cfg,
		mode:      mode,
		viewport:  viewport,
		trimValue: true,
	})
}

func (h *ShellHandler) prepareExecution(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*shellExecutionConfig, error) {
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

	// Size the hint to vars.Env (the OS environment, the dominant source); step.Env
	// is a handful of overrides and the map grows as needed. Summing both lengths
	// trips CodeQL's allocation-overflow query for no real benefit.
	envMap := make(map[string]string, len(vars.Env))
	for k, v := range vars.Env {
		envMap[k] = v
	}
	if len(step.Env) > 0 {
		resolvedEnv, err := vars.ResolveEnvMap(step.Env)
		if err != nil {
			return nil, fmt.Errorf("step '%s': %w", step.Name, err)
		}
		for k, v := range resolvedEnv {
			envMap[k] = v
		}
	}

	return &shellExecutionConfig{
		command: command,
		workDir: workDir,
		env:     envMapToSlice(envMap),
	}, nil
}

func (h *ShellHandler) executePrepared(ctx context.Context, req shellExecutionRequest) (*StepResult, error) {
	step := req.step
	cfg := req.config
	if step.Tty || step.Interactive {
		return h.executeShellSessionStep(ctx, step, cfg)
	}

	if executionOptionsFromContext(ctx).DryRun {
		return NewStepResult("").
			WithMetadata("stdout", "").
			WithMetadata("stderr", "").
			WithMetadata(exitCodeMetadata, 0), nil
	}

	cmd := process.NewShellCommand(ctx, cfg.command)
	if cfg.workDir != "" {
		cmd.Dir = cfg.workDir
	}
	cmd.Env = cfg.env

	writer := NewOutputModeWriter(req.mode, step.Name, req.viewport)
	stdout, stderr, err := writer.Execute(cmd)
	if err != nil {
		return NewStepResult(stdout).
			WithError(stderr).
			WithMetadata("stdout", stdout).
			WithMetadata("stderr", stderr).
			WithMetadata(exitCodeMetadata, getExitCode(err)), err
	}

	value := stdout
	if req.trimValue {
		value = strings.TrimSpace(stdout)
	}
	return NewStepResult(value).
		WithMetadata("stdout", stdout).
		WithMetadata("stderr", stderr).
		WithMetadata(exitCodeMetadata, 0), nil
}

func envMapToSlice(envMap map[string]string) []string {
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}
	return env
}
