package workflow

import (
	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	stepPkg "github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/samber/lo"
)

// workflowErrorTitle is the standard title for workflow errors.
const workflowErrorTitle = "Workflow Error"

// toolchainDocsURL is the documentation URL for toolchain configuration.
const toolchainDocsURL = "https://atmos.tools/cli/commands/toolchain/"

// logFieldStep is the structured-logging/error-context key used for the current step.
const logFieldStep = "step"

// mergeWorkflowEnv merges workflow and step environment variables.
// Step env overrides workflow env for same keys.
func mergeWorkflowEnv(workflowEnv, stepEnv map[string]string) map[string]string {
	if len(workflowEnv) == 0 && len(stepEnv) == 0 {
		return nil
	}
	merged := make(map[string]string, len(workflowEnv)+len(stepEnv))
	for k, v := range workflowEnv {
		merged[k] = v
	}
	for k, v := range stepEnv {
		merged[k] = v
	}
	return merged
}

// Executor handles workflow execution with dependency injection for testing.
type Executor struct {
	runner           CommandRunner
	authProvider     AuthProvider
	ui               UIProvider
	depProvider      DependencyProvider
	stepVars         *stepPkg.Variables
	containerSession *ContainerSession
}

// NewExecutor creates a new Executor with the given dependencies.
// Nil dependencies are handled gracefully: runner is required for command execution,
// authProvider can be nil if no authentication is needed, ui can be nil to
// disable user-facing output, and depProvider can be nil to skip toolchain integration.
func NewExecutor(runner CommandRunner, authProvider AuthProvider, ui UIProvider) *Executor {
	return &Executor{
		runner:       runner,
		authProvider: authProvider,
		ui:           ui,
		depProvider:  nil, // Will be set per-execute based on AtmosConfig.
	}
}

// WithDependencyProvider sets a custom DependencyProvider (primarily for testing).
func (e *Executor) WithDependencyProvider(provider DependencyProvider) *Executor {
	e.depProvider = provider
	return e
}

func (e *Executor) cleanupWorkflowContainer(success bool) error {
	if e.containerSession == nil {
		return nil
	}
	session := e.containerSession
	e.containerSession = nil
	return session.Cleanup(success)
}

// Execute runs a workflow with the given options.
// This is the main entry point for workflow execution.
func (e *Executor) Execute(params *WorkflowParams) (result *ExecutionResult, err error) {
	if params == nil || params.AtmosConfig == nil {
		return nil, errUtils.ErrNilParam
	}
	defer perf.Track(params.AtmosConfig, "workflow.Executor.Execute")()

	result = &ExecutionResult{
		WorkflowName: params.Workflow,
		Steps:        make([]StepResult, 0),
		Success:      true,
	}
	defer func() {
		if cleanupErr := e.cleanupWorkflowContainer(result.Success); cleanupErr != nil && result.Success {
			result.Success = false
			result.Error = cleanupErr
			err = cleanupErr
		}
	}()

	steps, err := e.prepareSteps(params, result)
	if err != nil {
		return result, err
	}

	// Initialize show renderer for header/flags display.
	showRenderer := NewShowRenderer()

	// Build flags map for header display.
	flags := e.buildFlagsMap(params)

	// Initialize progress renderer if enabled.
	progressRenderer := NewProgressRenderer(params.WorkflowDefinition, len(steps))

	// Render header before first step (if enabled).
	showRenderer.RenderHeaderIfNeeded(params.WorkflowDefinition, params.Workflow, flags)

	// Execute each step.
	if stepErr := e.runSteps(params, steps, progressRenderer, result); stepErr != nil {
		return result, stepErr
	}

	return result, nil
}

// prepareSteps validates the workflow, generates step names, applies --from-step,
// and ensures toolchain dependencies are installed before execution.
func (e *Executor) prepareSteps(params *WorkflowParams, result *ExecutionResult) ([]schema.WorkflowStep, error) {
	steps := params.WorkflowDefinition.Steps

	// Validate workflow has steps.
	if len(steps) == 0 {
		err := errUtils.Build(errUtils.ErrWorkflowNoSteps).
			WithExplanationf("Workflow `%s` is empty and requires at least one step to execute.", params.Workflow).
			Err()
		e.printError(err)
		result.Success = false
		result.Error = err
		return nil, err
	}

	// Generate step names if not provided.
	CheckAndGenerateWorkflowStepNames(params.WorkflowDefinition)
	e.stepVars = stepPkg.NewVariables()

	// Validate control steps (parallel/matrix) and their nested step constraints.
	if err := schema.ValidateWorkflowSteps(params.WorkflowDefinition.Steps); err != nil {
		e.printError(err)
		result.Success = false
		result.Error = err
		return nil, err
	}

	log.Debug("Executing workflow", "workflow", params.Workflow, "path", params.WorkflowPath)

	// Handle --from-step flag.
	steps, err := e.handleFromStep(steps, params.WorkflowDefinition, params.Workflow, params.Opts.FromStep, result)
	if err != nil {
		return nil, err
	}

	// Ensure toolchain dependencies are installed and PATH is updated.
	if err := e.ensureToolchainDependencies(params); err != nil {
		e.printError(err)
		result.Success = false
		result.Error = err
		return nil, err
	}

	return steps, nil
}

// runSteps executes each workflow step in order, updating result. It returns the
// first failing step's error (if any) and marks progress as done.
func (e *Executor) runSteps(params *WorkflowParams, steps []schema.WorkflowStep, progressRenderer *ProgressRenderer, result *ExecutionResult) error {
	conditionStatus := schema.ConditionPredicateSuccess
	for stepIdx := range steps {
		step := &steps[stepIdx]
		if err := schema.ValidateStepCondition(step.When); err != nil {
			result.Success = false
			result.Error = err
			return err
		}
		conditionContext := workflowConditionContext(params.Workflow, params.WorkflowDefinition, step, params.Opts.CommandLineStack)
		conditionContext.Status = conditionStatus
		runs, err := step.When.EvaluateWithImplicitSuccessE(conditionContext)
		if err != nil {
			result.Success = false
			result.Error = err
			return err
		}
		if !runs {
			log.Debug("Skipping workflow step, `when` condition did not match", logFieldStep, step.Name)
			result.Steps = append(result.Steps, StepResult{
				StepName: step.Name,
				Command:  step.Command,
				Success:  true,
				Skipped:  true,
			})
			continue
		}
		// Update and render progress (if enabled).
		if progressRenderer.IsEnabled() {
			progressRenderer.Update(stepIdx+1, step.Name)
			progressRenderer.Render()
		}

		// Wrap each step's output in a collapsible CI log group when grouping is
		// active. Exec steps run bare because a successful Unix exec never returns
		// to close a deferred group.
		var stepResult stepResultInternal
		_ = stepPkg.RunGroupedForType(params.AtmosConfig, step.Name, step.Command, step.Type, func() error {
			stepResult = e.executeStep(params, step, stepIdx)
			return nil
		})
		result.Steps = append(result.Steps, stepResult.StepResult)

		if !stepResult.Success {
			result.Success = false
			result.Error = stepResult.Error
			result.ResumeCommand = e.buildResumeCommand(params.Workflow, params.WorkflowPath, step.Name, stepResult.finalStack, params.AtmosConfig)
			if progressRenderer.IsEnabled() {
				progressRenderer.Done()
			}
			return stepResult.Error
		}
	}

	// Mark progress as done.
	if progressRenderer.IsEnabled() {
		progressRenderer.Done()
	}

	return nil
}

func workflowConditionContext(workflow string, workflowDefinition *schema.WorkflowDefinition, step *schema.WorkflowStep, commandLineStack string) schema.ConditionContext {
	return BuildConditionContext(workflow, workflowDefinition, step, commandLineStack, nil)
}

// BuildConditionContext constructs the runtime facts exposed to workflow `when`
// conditions. Step stack overrides workflow stack, command-line stack overrides
// both, and step env overlays workflow/base env.
func BuildConditionContext(workflow string, workflowDefinition *schema.WorkflowDefinition, step *schema.WorkflowStep, commandLineStack string, baseEnv map[string]string) schema.ConditionContext {
	defer perf.Track(nil, "workflow.BuildConditionContext")()

	stack := ""
	stepName := ""
	env := baseEnv
	if workflowDefinition != nil {
		stack = workflowDefinition.Stack
		if env == nil {
			env = workflowDefinition.Env
		}
	}
	if step != nil {
		if step.Stack != "" {
			stack = step.Stack
		}
		stepName = step.Name
		if len(step.Env) > 0 {
			merged := make(map[string]string, len(env))
			for key, value := range env {
				merged[key] = value
			}
			for key, value := range step.Env {
				merged[key] = value
			}
			env = merged
		}
	}
	if commandLineStack != "" {
		stack = commandLineStack
	}
	return schema.ConditionContext{
		CI:       telemetry.IsCI(),
		Status:   schema.ConditionPredicateSuccess,
		Stack:    stack,
		Workflow: workflow,
		Step:     stepName,
		Env:      env,
	}
}

// buildFlagsMap builds a map of flags for display in the header.
func (e *Executor) buildFlagsMap(params *WorkflowParams) map[string]string {
	flags := make(map[string]string)
	if params.Opts.CommandLineStack != "" {
		flags["stack"] = params.Opts.CommandLineStack
	}
	if params.Opts.CommandLineIdentity != "" {
		flags["identity"] = params.Opts.CommandLineIdentity
	}
	if params.Opts.DryRun {
		flags["dry-run"] = "true"
	}
	if params.Opts.FromStep != "" {
		flags["from-step"] = params.Opts.FromStep
	}
	return flags
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
			WithExplanationf("The `--from-step` flag was set to `%s`, but this step does not exist in workflow `%s`.\n\n### Available steps:\n\n%s", fromStep, workflow, u.FormatList(stepNames)).
			Err()
		e.printError(err)
		result.Success = false
		result.Error = err
		return nil, err
	}

	// Mark skipped steps in result.
	for i := range workflowDefinition.Steps {
		step := &workflowDefinition.Steps[i]
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
