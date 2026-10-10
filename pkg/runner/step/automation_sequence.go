package step

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// RunSteps runs a synchronous sequence with shared outputs and environment.
// Scheduler-only policies are rejected before any step executes.
func (l *AutomationLibrary) RunSteps(ctx context.Context, tasks schema.Tasks, call *automation.StepCall) error {
	defer perf.Track(nil, "step.AutomationLibrary.RunSteps")()
	if call == nil {
		return fmt.Errorf("%w: step execution settings are required", errUtils.ErrAutomation)
	}
	steps, err := prepareAutomationSteps(tasks, call.Parallel || l.vars.automationParallel)
	if err != nil {
		return err
	}
	for i := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := l.runStep(ctx, &steps[i], call); err != nil {
			return fmt.Errorf("step %q: %w", steps[i].Name, err)
		}
	}
	return nil
}

// ValidateSteps checks a step sequence without running it: duplicate names, unknown step types,
// unsupported fields, and scheduler-only policies are rejected exactly as RunSteps rejects them.
// Callers use it to fail early, for example when installing Git hook shims.
func (l *AutomationLibrary) ValidateSteps(tasks schema.Tasks) error {
	defer perf.Track(nil, "step.AutomationLibrary.ValidateSteps")()

	_, err := prepareAutomationSteps(tasks, l.vars.automationParallel)
	return err
}

func prepareAutomationSteps(tasks schema.Tasks, parallel bool) ([]schema.WorkflowStep, error) {
	steps := make([]schema.WorkflowStep, len(tasks))
	names := make(map[string]bool, len(tasks))
	for i := range tasks {
		steps[i] = tasks[i].ToWorkflowStep()
		if steps[i].Name == "" {
			steps[i].Name = fmt.Sprintf("step_%d", i+1)
		}
		if steps[i].Type == "" {
			steps[i].Type = schema.TaskTypeShell
		}
		if names[steps[i].Name] {
			return nil, fmt.Errorf("%w: duplicate step name %q", errUtils.ErrAutomation, steps[i].Name)
		}
		names[steps[i].Name] = true
		if err := validateAutomationStep(&steps[i], parallel); err != nil {
			return nil, fmt.Errorf("step %q: %w", steps[i].Name, err)
		}
	}
	return steps, nil
}
