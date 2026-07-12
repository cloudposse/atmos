package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/retry"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
)

// resolveStepIdentity determines the identity to authenticate as for a step,
// falling back to the command-line identity when the step doesn't specify one.
func resolveStepIdentity(step *schema.WorkflowStep, params *WorkflowParams) string {
	stepIdentity := strings.TrimSpace(step.Identity)
	if stepIdentity == "" && params.Opts.CommandLineIdentity != "" {
		stepIdentity = params.Opts.CommandLineIdentity
	}
	return stepIdentity
}

// finalizeStepCommand normalizes the step's command type (defaulting to atmos)
// and, for script steps, renders the display form of the command.
func finalizeStepCommand(commandType, command string, step *schema.WorkflowStep) (string, string) {
	if commandType == "" {
		commandType = "atmos"
	}
	if commandType == schema.TaskTypeScript {
		command = process.FormatScriptDisplay(step.Interpreter, step.Script)
	}
	return commandType, command
}

// shouldRunScriptInContainer reports whether a script-type step must run inside
// a workflow or step-level container.
func shouldRunScriptInContainer(commandType string, step *schema.WorkflowStep, workflowDef *schema.WorkflowDefinition) bool {
	if commandType != schema.TaskTypeScript {
		return false
	}
	return StepContainerOverride(step) || (workflowDef.Container != nil && workflowDef.Container.IsEnabled() && !StepContainerDisabled(step))
}

// recordStepOutputs stores the step's exit-code metadata into the executor's
// variable scope so downstream steps can reference its outputs.
func (e *Executor) recordStepOutputs(step *schema.WorkflowStep) {
	if e.stepVars != nil {
		_ = e.stepVars.SetWithOutputs(step.Name, stepPkg.NewStepResult("").WithMetadata("exit_code", 0), step.Outputs)
	}
}

// successResult builds a successful stepResultInternal for the given step.
func (e *Executor) successResult(step *schema.WorkflowStep, command, finalStack string) stepResultInternal {
	return stepResultInternal{
		StepResult: StepResult{
			StepName: step.Name,
			Command:  command,
			Success:  true,
		},
		finalStack: finalStack,
	}
}

// executeStep executes a single workflow step and returns its result.
func (e *Executor) executeStep(params *WorkflowParams, step *schema.WorkflowStep, stepIdx int) stepResultInternal {
	command := strings.TrimSpace(step.Command)
	commandType := strings.TrimSpace(step.Type)
	log.Debug("Executing workflow step", logFieldStep, stepIdx, "name", step.Name, "command", command)
	commandType, command = finalizeStepCommand(commandType, command, step)
	cmdParams, err := e.prepareStepExecution(params, step, stepIdx, command, commandType)
	if err != nil {
		return stepResultInternal{StepResult: StepResult{StepName: step.Name, Command: command, Success: false, Error: err}}
	}
	if shouldRunScriptInContainer(commandType, step, params.WorkflowDefinition) {
		if err := e.runShellStep(params, step, cmdParams, cmdParams.workingDirectory); err != nil {
			return e.handleStepError(params, step.Name, cmdParams, err)
		}
		e.recordStepOutputs(step)
		return e.successResult(step, command, cmdParams.finalStack)
	}
	if stepPkg.IsExtendedStepType(commandType) {
		return e.executeRegisteredStep(params, step, cmdParams)
	}
	if err := e.runCommand(params, step, cmdParams); err != nil {
		return e.handleStepError(params, step.Name, cmdParams, err)
	}
	e.recordStepOutputs(step)
	return e.successResult(step, command, cmdParams.finalStack)
}

// prepareStepExecution resolves the environment, stack, working directory, and
// rendered command for a step, returning the parameters needed to run it.
func (e *Executor) prepareStepExecution(
	params *WorkflowParams, step *schema.WorkflowStep, stepIdx int, command, commandType string,
) (*runCommandParams, error) {
	stepEnv, err := e.prepareStepEnvironment(
		params.Ctx, resolveStepIdentity(step, params), step.Name,
		params.WorkflowDefinition.Env, step.Env,
	)
	if err != nil {
		return nil, err
	}
	if commandType != schema.TaskTypeExec && ci.ShouldPropagateLogGroupSentinel(params.AtmosConfig, ci.DimensionStep) {
		stepEnv = append(stepEnv, ci.LogGroupSentinelEnv())
	}
	finalStack := e.calculateFinalStack(params.WorkflowDefinition, step, params.Opts.CommandLineStack)
	workDir := e.calculateWorkingDirectory(params.WorkflowDefinition, step, params.AtmosConfig.BasePath)
	e.renderStepCommand(step, params.WorkflowDefinition, command, commandType, finalStack)
	return &runCommandParams{
		command: command, commandType: commandType, stepIdx: stepIdx,
		finalStack: finalStack, stepEnv: stepEnv, workingDirectory: workDir,
	}, nil
}

// executeRegisteredStep executes non-legacy workflow step types via the shared step registry.
func (e *Executor) executeRegisteredStep(params *WorkflowParams, step *schema.WorkflowStep, cmdParams *runCommandParams) stepResultInternal {
	handler, ok := stepPkg.Get(cmdParams.commandType)
	if !ok {
		err := errUtils.Build(errUtils.ErrUnknownStepType).
			WithContext("step", step.Name).
			WithContext("type", cmdParams.commandType).
			Err()
		return e.handleStepError(params, step.Name, cmdParams, err)
	}

	stepCopy := *step
	stepCopy.WorkingDirectory = cmdParams.workingDirectory
	stepCopy.Env = envSliceToMap(cmdParams.stepEnv)
	stepCopy.DryRun = params.Opts.DryRun
	stepCopy.Stack = cmdParams.finalStack

	if err := handler.Validate(&stepCopy); err != nil {
		return e.handleStepError(params, step.Name, cmdParams, err)
	}

	if e.stepVars == nil {
		e.stepVars = stepPkg.NewVariables()
	}
	// Always set (or clear) the stack flag so a stackless step does not
	// inherit a stale value from a prior step in the same workflow.
	e.stepVars.SetFlag("stack", cmdParams.finalStack)

	stepCtx, cancel, err := resolveStepContext(params.Ctx, stepCopy.Timeout)
	if err != nil {
		return e.handleStepError(params, step.Name, cmdParams, err)
	}
	if cancel != nil {
		defer cancel()
	}

	if _, err := e.runStepWithRetry(stepCtx, handler, &stepCopy, params.WorkflowDefinition); err != nil {
		return e.handleStepError(params, step.Name, cmdParams, err)
	}

	return stepResultInternal{
		StepResult: StepResult{
			StepName: step.Name,
			Command:  cmdParams.command,
			Success:  true,
		},
		finalStack: cmdParams.finalStack,
	}
}

// resolveStepContext derives the execution context for a step, applying an
// optional timeout. The returned cancel func is nil when no timeout applies.
func resolveStepContext(ctx context.Context, timeoutSpec string) (context.Context, context.CancelFunc, error) {
	if timeoutSpec == "" {
		return ctx, nil, nil
	}
	timeout, parseErr := time.ParseDuration(timeoutSpec)
	if parseErr != nil {
		return nil, nil, parseErr
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	return stepCtx, cancel, nil
}

// runStepWithRetry executes a step handler, honoring its retry policy, and
// records the step outputs into the executor's variable scope.
func (e *Executor) runStepWithRetry(stepCtx context.Context, handler stepPkg.StepHandler, stepCopy *schema.WorkflowStep, workflow *schema.WorkflowDefinition) (*stepPkg.StepResult, error) {
	var result *stepPkg.StepResult
	execute := func() error {
		var execErr error
		result, execErr = executeStepHandlerWithWorkflow(stepCtx, handler, stepCopy, e.stepVars, workflow)
		return execErr
	}

	var err error
	if stepCopy.Retry != nil {
		err = retry.Do(stepCtx, stepCopy.Retry, execute)
	} else {
		err = execute()
	}
	if result != nil {
		if outputErr := e.stepVars.SetWithOutputs(stepCopy.Name, result, stepCopy.Outputs); outputErr != nil && err == nil {
			err = outputErr
		}
	}
	return result, err
}

func executeStepHandlerWithWorkflow(
	ctx context.Context,
	handler stepPkg.StepHandler,
	step *schema.WorkflowStep,
	vars *stepPkg.Variables,
	workflow *schema.WorkflowDefinition,
) (*stepPkg.StepResult, error) {
	type workflowAwareHandler interface {
		ExecuteWithWorkflow(context.Context, *schema.WorkflowStep, *stepPkg.Variables, *schema.WorkflowDefinition) (*stepPkg.StepResult, error)
	}
	if wah, ok := handler.(workflowAwareHandler); ok {
		return wah.ExecuteWithWorkflow(ctx, step, vars, workflow)
	}
	return handler.Execute(ctx, step, vars)
}

func envSliceToMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	result := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		result[key] = value
	}
	return result
}

// renderStepCommand renders the command before execution if show.command is enabled.
func (e *Executor) renderStepCommand(step *schema.WorkflowStep, workflow *schema.WorkflowDefinition, command, commandType, finalStack string) {
	displayCmd := command
	if commandType == config.AtmosCommand && finalStack != "" {
		displayCmd = fmt.Sprintf("atmos %s -s %s", command, finalStack)
	} else if commandType == config.AtmosCommand {
		displayCmd = "atmos " + command
	}
	stepPkg.RenderCommand(step, workflow, displayCmd)
}
