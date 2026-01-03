package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StepExecutor runs workflow steps using the step registry.
// This provides a simplified interface for executing steps with variable passing.
type StepExecutor struct {
	vars     *Variables
	workflow *schema.WorkflowDefinition
}

// NewStepExecutor creates a new step executor.
func NewStepExecutor() *StepExecutor {
	defer perf.Track(nil, "step.NewStepExecutor")()

	return &StepExecutor{
		vars: NewVariables(),
	}
}

// NewStepExecutorWithVars creates a new executor with pre-populated variables.
func NewStepExecutorWithVars(vars *Variables) *StepExecutor {
	defer perf.Track(nil, "step.NewStepExecutorWithVars")()

	return &StepExecutor{
		vars: vars,
	}
}

// SetWorkflow sets the workflow context for output mode inheritance.
func (e *StepExecutor) SetWorkflow(workflow *schema.WorkflowDefinition) {
	defer perf.Track(nil, "step.StepExecutor.SetWorkflow")()

	e.workflow = workflow
}

// Variables returns the executor's variable store.
func (e *StepExecutor) Variables() *Variables {
	defer perf.Track(nil, "step.StepExecutor.Variables")()

	return e.vars
}

// Execute runs a single step and stores the result.
func (e *StepExecutor) Execute(ctx context.Context, step *schema.WorkflowStep) (*StepResult, error) {
	defer perf.Track(nil, "step.StepExecutor.Execute")()

	// Default step name if not provided.
	if step.Name == "" {
		step.Name = "unnamed_step"
	}

	// Default step type if not provided.
	if step.Type == "" {
		step.Type = "shell"
	}

	// Get handler from registry.
	handler, ok := Get(step.Type)
	if !ok {
		return nil, errUtils.Build(errUtils.ErrUnknownStepType).
			WithContext("step", step.Name).
			WithContext("type", step.Type).
			Err()
	}

	// Validate step configuration.
	if err := handler.Validate(step); err != nil {
		return nil, err
	}

	// Execute with workflow context if available.
	var result *StepResult
	var err error

	if e.workflow != nil {
		result, err = e.executeWithWorkflowContext(ctx, handler, step)
	} else {
		result, err = handler.Execute(ctx, step, e.vars)
	}

	if err != nil {
		return result, err
	}

	// Store result for variable access.
	e.vars.Set(step.Name, result)

	return result, nil
}

// executeWithWorkflowContext runs a step with workflow-level settings.
func (e *StepExecutor) executeWithWorkflowContext(ctx context.Context, handler StepHandler, step *schema.WorkflowStep) (*StepResult, error) {
	// Check if handler supports workflow context (for output mode inheritance).
	type workflowAwareHandler interface {
		ExecuteWithWorkflow(ctx context.Context, step *schema.WorkflowStep, vars *Variables, workflow *schema.WorkflowDefinition) (*StepResult, error)
	}

	if wah, ok := handler.(workflowAwareHandler); ok {
		return wah.ExecuteWithWorkflow(ctx, step, e.vars, e.workflow)
	}

	// Fall back to standard execution.
	return handler.Execute(ctx, step, e.vars)
}

// RunAll executes all steps in order.
func (e *StepExecutor) RunAll(ctx context.Context, workflow *schema.WorkflowDefinition) error {
	defer perf.Track(nil, "step.StepExecutor.RunAll")()

	e.workflow = workflow

	for i := range workflow.Steps {
		step := &workflow.Steps[i]
		if step.Name == "" {
			step.Name = fmt.Sprintf("step_%d", i+1)
		}

		_, err := e.Execute(ctx, step)
		if err != nil {
			return fmt.Errorf("step '%s': %w: %w", step.Name, errUtils.ErrWorkflowStepFailed, err)
		}
	}

	return nil
}

// GetResult returns the result of a previously executed step.
func (e *StepExecutor) GetResult(name string) (*StepResult, bool) {
	defer perf.Track(nil, "step.StepExecutor.GetResult")()

	result, ok := e.vars.Steps[name]
	return result, ok
}

// SetEnv sets an environment variable for use in templates.
func (e *StepExecutor) SetEnv(key, value string) {
	defer perf.Track(nil, "step.StepExecutor.SetEnv")()

	e.vars.SetEnv(key, value)
}

// IsExtendedStepType checks if a step type is an extended type (not atmos or shell).
func IsExtendedStepType(stepType string) bool {
	defer perf.Track(nil, "step.IsExtendedStepType")()

	// Legacy types that should be handled by existing executor.
	legacyTypes := map[string]bool{
		"atmos": true,
		"shell": true,
		"":      true, // Empty defaults to shell in legacy.
	}

	if legacyTypes[stepType] {
		return false
	}

	// Check if the type is registered.
	_, ok := Get(stepType)
	return ok
}

// ValidateStep validates a step configuration.
func ValidateStep(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.ValidateStep")()

	stepType := step.Type
	if stepType == "" {
		stepType = "shell"
	}

	handler, ok := Get(stepType)
	if !ok {
		return errUtils.Build(errUtils.ErrUnknownStepType).
			WithContext("step", step.Name).
			WithContext("type", stepType).
			Err()
	}

	return handler.Validate(step)
}

// ListTypes returns all available step type names grouped by category.
func ListTypes() map[StepCategory][]string {
	defer perf.Track(nil, "step.ListTypes")()

	byCategory := ListByCategory()
	result := make(map[StepCategory][]string)

	for cat, handlers := range byCategory {
		names := make([]string, 0, len(handlers))
		for _, h := range handlers {
			names = append(names, h.GetName())
		}
		result[cat] = names
	}

	return result
}

// ValidateWorkflow validates all steps in a workflow definition.
func ValidateWorkflow(workflow *schema.WorkflowDefinition) []error {
	defer perf.Track(nil, "step.ValidateWorkflow")()

	var errs []error

	for i := range workflow.Steps {
		step := &workflow.Steps[i]

		// Default step name if not provided.
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("step_%d", i+1)
		}

		if err := ValidateStep(step); err != nil {
			errs = append(errs, fmt.Errorf("step '%s': %w", name, err))
		}
	}

	return errs
}
