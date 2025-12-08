package workflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/samber/lo"
	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// workflowErrorTitle is the standard title for workflow errors.
const workflowErrorTitle = "Workflow Error"

// Executor handles workflow execution with dependency injection for testing.
type Executor struct {
	runner       CommandRunner
	authProvider AuthProvider
	ui           UIProvider
}

// NewExecutor creates a new Executor with the given dependencies.
// Nil dependencies are handled gracefully: runner is required for command execution,
// authProvider can be nil if no authentication is needed, and ui can be nil to
// disable user-facing output (messages and errors will be silently skipped).
func NewExecutor(runner CommandRunner, authProvider AuthProvider, ui UIProvider) *Executor {
	return &Executor{
		runner:       runner,
		authProvider: authProvider,
		ui:           ui,
	}
}

// Execute runs a workflow with the given options.
// This is the main entry point for workflow execution.
func (e *Executor) Execute(params *WorkflowParams) (*ExecutionResult, error) {
	if params == nil || params.AtmosConfig == nil {
		return nil, errUtils.ErrNilParam
	}
	defer perf.Track(params.AtmosConfig, "workflow.Executor.Execute")()

	result := &ExecutionResult{
		WorkflowName: params.Workflow,
		Steps:        make([]StepResult, 0),
		Success:      true,
	}

	steps := params.WorkflowDefinition.Steps

	// Validate workflow has steps.
	if len(steps) == 0 {
		err := errUtils.Build(errUtils.ErrWorkflowNoSteps).
			WithExplanationf("Workflow `%s` is empty and requires at least one step to execute.", params.Workflow).
			Err()
		e.printError(err)
		result.Success = false
		result.Error = err
		return result, err
	}

	// Generate step names if not provided.
	checkAndGenerateWorkflowStepNames(params.WorkflowDefinition)

	log.Debug("Executing workflow", "workflow", params.Workflow, "path", params.WorkflowPath)

	// Handle --from-step flag.
	steps, err := e.handleFromStep(steps, params.WorkflowDefinition, params.Workflow, params.Opts.FromStep, result)
	if err != nil {
		return result, err
	}

	// Execute each step.
	for stepIdx, step := range steps {
		stepResult := e.executeStep(params, &step, stepIdx)
		result.Steps = append(result.Steps, stepResult.StepResult)

		if !stepResult.Success {
			result.Success = false
			result.Error = stepResult.Error
			result.ResumeCommand = e.buildResumeCommand(params.Workflow, params.WorkflowPath, step.Name, stepResult.finalStack, params.AtmosConfig)
			return result, stepResult.Error
		}
	}

	return result, nil
}

// handleFromStep processes the --from-step flag and returns the filtered steps.
func (e *Executor) handleFromStep(
	steps []schema.WorkflowStep,
	workflowDefinition *schema.WorkflowDefinition,
	workflow string,
	fromStep string,
	result *ExecutionResult,
) ([]schema.WorkflowStep, error) {
	if fromStep == "" {
		return steps, nil
	}

	steps = lo.DropWhile[schema.WorkflowStep](steps, func(step schema.WorkflowStep) bool {
		return step.Name != fromStep
	})

	if len(steps) == 0 {
		stepNames := lo.Map(workflowDefinition.Steps, func(step schema.WorkflowStep, _ int) string { return step.Name })
		err := errUtils.Build(errUtils.ErrInvalidFromStep).
			WithExplanationf("The `--from-step` flag was set to `%s`, but this step does not exist in workflow `%s`.", fromStep, workflow).
			WithHintf("Available steps:\n%s", FormatList(stepNames)).
			Err()
		e.printError(err)
		result.Success = false
		result.Error = err
		return nil, err
	}

	// Mark skipped steps in result.
	for _, step := range workflowDefinition.Steps {
		if step.Name == fromStep {
			break
		}
		result.Steps = append(result.Steps, StepResult{
			StepName: step.Name,
			Command:  step.Command,
			Skipped:  true,
			Success:  true,
		})
	}

	return steps, nil
}

// stepResultInternal extends StepResult with internal fields.
type stepResultInternal struct {
	StepResult
	finalStack string
}

// executeStep executes a single workflow step.
func (e *Executor) executeStep(params *WorkflowParams, step *schema.WorkflowStep, stepIdx int) stepResultInternal {
	command := strings.TrimSpace(step.Command)
	commandType := strings.TrimSpace(step.Type)
	stepIdentity := strings.TrimSpace(step.Identity)

	// Use command-line identity if step doesn't specify one.
	if stepIdentity == "" && params.Opts.CommandLineIdentity != "" {
		stepIdentity = params.Opts.CommandLineIdentity
	}

	log.Debug("Executing workflow step", "step", stepIdx, "name", step.Name, "command", command)

	if commandType == "" {
		commandType = "atmos"
	}

	// Prepare environment with authentication if needed.
	stepEnv, err := e.prepareStepEnvironment(params.Ctx, stepIdentity, step.Name)
	if err != nil {
		return stepResultInternal{
			StepResult: StepResult{
				StepName: step.Name,
				Command:  command,
				Success:  false,
				Error:    err,
			},
		}
	}

	// Calculate final stack.
	finalStack := e.calculateFinalStack(params.WorkflowDefinition, step, params.Opts.CommandLineStack)

	// Execute the command based on type.
	cmdParams := &runCommandParams{
		command:     command,
		commandType: commandType,
		stepIdx:     stepIdx,
		finalStack:  finalStack,
		stepEnv:     stepEnv,
	}
	err = e.runCommand(params, cmdParams)
	if err != nil {
		return e.handleStepError(params, step.Name, cmdParams, err)
	}

	return stepResultInternal{
		StepResult: StepResult{
			StepName: step.Name,
			Command:  command,
			Success:  true,
		},
		finalStack: finalStack,
	}
}

// prepareStepEnvironment prepares the environment for a step, handling authentication if needed.
func (e *Executor) prepareStepEnvironment(ctx context.Context, stepIdentity, stepName string) ([]string, error) {
	if stepIdentity == "" || e.authProvider == nil {
		return []string{}, nil
	}
	return e.prepareAuthenticatedEnvironment(ctx, stepIdentity, stepName)
}

// runCommandParams holds parameters for command execution.
type runCommandParams struct {
	command     string
	commandType string
	stepIdx     int
	finalStack  string
	stepEnv     []string
}

// runCommand executes the appropriate command type.
func (e *Executor) runCommand(params *WorkflowParams, cmdParams *runCommandParams) error {
	switch cmdParams.commandType {
	case "shell":
		commandName := fmt.Sprintf("%s-step-%d", params.Workflow, cmdParams.stepIdx)
		return e.runner.RunShell(cmdParams.command, commandName, ".", cmdParams.stepEnv, params.Opts.DryRun)
	case "atmos":
		return e.executeAtmosCommand(params, cmdParams.command, cmdParams.finalStack, cmdParams.stepEnv)
	default:
		// Return error without printing - handleStepError will print it with resume context.
		return errUtils.Build(errUtils.ErrInvalidWorkflowStepType).
			WithExplanationf("Step type `%s` is not supported. Each step must specify a valid type.", cmdParams.commandType).
			WithHintf("Available types:\n%s", FormatList([]string{"atmos", "shell"})).
			Err()
	}
}

// handleStepError handles a step execution error and returns the appropriate result.
func (e *Executor) handleStepError(params *WorkflowParams, stepName string, cmdParams *runCommandParams, err error) stepResultInternal {
	log.Debug("Workflow step failed", "step", stepName, "error", err)

	// Build resume command for all error types.
	resumeCmd := e.buildResumeCommand(params.Workflow, params.WorkflowPath, stepName, cmdParams.finalStack, params.AtmosConfig)

	// For workflow-specific errors (like invalid step type), add resume hint and print directly
	// without wrapping in ErrWorkflowStepFailed - they already have their own context.
	if errors.Is(err, errUtils.ErrInvalidWorkflowStepType) {
		// Add resume command hint to the existing error.
		enrichedErr := errUtils.Build(err).
			WithHintf("To resume the workflow from this step, run:\n```\n%s\n```", resumeCmd).
			Err()
		e.printError(enrichedErr)
		return stepResultInternal{
			StepResult: StepResult{
				StepName: stepName,
				Command:  cmdParams.command,
				Success:  false,
				Error:    enrichedErr,
			},
			finalStack: cmdParams.finalStack,
		}
	}

	// Build failed command string for error message.
	failedCmd := cmdParams.command
	if cmdParams.commandType == config.AtmosCommand {
		failedCmd = config.AtmosCommand + " " + cmdParams.command
		if cmdParams.finalStack != "" {
			failedCmd = fmt.Sprintf("%s -s %s", failedCmd, cmdParams.finalStack)
		}
	}

	stepErr := errUtils.Build(errUtils.ErrWorkflowStepFailed).
		WithCause(err).
		WithExplanationf("The following command failed to execute:\n```\n%s\n```", failedCmd).
		WithHintf("To resume the workflow from this step, run:\n```\n%s\n```", resumeCmd).
		Err()
	e.printError(stepErr)

	return stepResultInternal{
		StepResult: StepResult{
			StepName: stepName,
			Command:  cmdParams.command,
			Success:  false,
			Error:    stepErr,
		},
		finalStack: cmdParams.finalStack,
	}
}

// prepareAuthenticatedEnvironment prepares environment variables with authentication.
func (e *Executor) prepareAuthenticatedEnvironment(ctx context.Context, identity, stepName string) ([]string, error) {
	if e.authProvider == nil {
		return nil, fmt.Errorf("%w: identity %q specified for step %q", errUtils.ErrAuthProviderNotAvailable, identity, stepName)
	}

	// Try cached credentials first.
	_, err := e.authProvider.GetCachedCredentials(ctx, identity)
	if err != nil {
		log.Debug("No valid cached credentials found, authenticating", "identity", identity, "error", err)
		err = e.authProvider.Authenticate(ctx, identity)
		if err != nil {
			if errors.Is(err, errUtils.ErrUserAborted) {
				return nil, errUtils.ErrUserAborted
			}
			return nil, fmt.Errorf("%w for identity %q in step %q: %w", errUtils.ErrAuthenticationFailed, identity, stepName, err)
		}
	}

	// Prepare environment.
	stepEnv, err := e.authProvider.PrepareEnvironment(ctx, identity, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to prepare shell environment for identity %q in step %q: %w", errUtils.ErrAuthenticationFailed, identity, stepName, err)
	}

	log.Debug("Prepared environment with identity", "identity", identity, "step", stepName)
	return stepEnv, nil
}

// executeAtmosCommand executes an atmos command with the given parameters.
func (e *Executor) executeAtmosCommand(params *WorkflowParams, command, finalStack string, stepEnv []string) error {
	// Parse command using shell.Fields for proper quote handling.
	args, parseErr := shell.Fields(command, nil)
	if parseErr != nil {
		log.Debug("Shell parsing failed, falling back to strings.Fields", "error", parseErr, "command", command)
		args = strings.Fields(command)
	}

	// Add stack argument if specified.
	if finalStack != "" {
		if idx := slices.Index(args, "--"); idx != -1 {
			args = append(args[:idx], append([]string{"-s", finalStack}, args[idx:]...)...)
		} else {
			args = append(args, "-s", finalStack)
		}
		log.Debug("Using stack", "stack", finalStack)
	}

	e.printMessage("Executing command: `atmos %s`\n", command)
	execParams := &AtmosExecParams{
		Ctx:         params.Ctx,
		AtmosConfig: params.AtmosConfig,
		Args:        args,
		Dir:         ".",
		Env:         stepEnv,
		DryRun:      params.Opts.DryRun,
	}
	return e.runner.RunAtmos(execParams)
}

// calculateFinalStack determines the final stack based on precedence rules.
func (e *Executor) calculateFinalStack(workflowDef *schema.WorkflowDefinition, step *schema.WorkflowStep, commandLineStack string) string {
	finalStack := ""
	workflowStack := strings.TrimSpace(workflowDef.Stack)
	stepStack := strings.TrimSpace(step.Stack)

	// Precedence: command-line > step > workflow.
	if workflowStack != "" {
		finalStack = workflowStack
	}
	if stepStack != "" {
		finalStack = stepStack
	}
	if commandLineStack != "" {
		finalStack = commandLineStack
	}

	return finalStack
}

// buildResumeCommand builds a command to resume the workflow from a specific step.
func (e *Executor) buildResumeCommand(workflow, workflowPath, stepName, finalStack string, atmosConfig *schema.AtmosConfiguration) string {
	workflowFileName := strings.TrimPrefix(filepath.ToSlash(workflowPath), filepath.ToSlash(atmosConfig.Workflows.BasePath))
	workflowFileName = strings.TrimPrefix(workflowFileName, "/")
	workflowFileName = strings.TrimSuffix(workflowFileName, filepath.Ext(workflowFileName))

	resumeCommand := fmt.Sprintf("%s workflow %s -f %s --from-step %s", config.AtmosCommand, workflow, workflowFileName, stepName)
	if finalStack != "" {
		resumeCommand = fmt.Sprintf("%s -s %s", resumeCommand, finalStack)
	}
	return resumeCommand
}

// printMessage prints a message using the UI provider.
func (e *Executor) printMessage(format string, args ...any) {
	if e.ui != nil {
		e.ui.PrintMessage(format, args...)
	}
}

// printError prints an error using the UI provider with the standard workflow error title.
// The error should be built using ErrorBuilder with explanation and hints included.
func (e *Executor) printError(err error) {
	if e.ui != nil {
		// Pass empty explanation since it's now embedded in the error via ErrorBuilder.
		e.ui.PrintError(err, workflowErrorTitle, "")
	}
}

// checkAndGenerateWorkflowStepNames generates step names for steps that don't have them.
func checkAndGenerateWorkflowStepNames(workflowDefinition *schema.WorkflowDefinition) {
	steps := workflowDefinition.Steps
	if steps == nil {
		return
	}

	for index, step := range steps {
		if step.Name == "" {
			steps[index].Name = fmt.Sprintf("step%d", index+1)
		}
	}
}

// FormatList formats a list of strings into a markdown bullet list.
func FormatList(items []string) string {
	var result strings.Builder
	for _, item := range items {
		result.WriteString(fmt.Sprintf("- `%s`\n", item))
	}
	return result.String()
}
