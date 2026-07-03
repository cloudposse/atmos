package schema

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors for exec step validation.
var (
	// ErrExecStepNotLast is returned when a step of type exec is not the final step.
	ErrExecStepNotLast = errors.New("exec step must be the last step (the process is replaced; later steps would never run)")
	// ErrExecStepInvalidField is returned when an exec step sets a field that
	// is meaningless after process replacement.
	ErrExecStepInvalidField = errors.New("field is not supported on exec steps (the process is replaced)")
	// ErrWorkflowControlStepInvalid is returned when a parallel or matrix step is misconfigured.
	ErrWorkflowControlStepInvalid = errors.New("invalid workflow control step")
)

// execStepView is the type-independent projection of a step used by exec validation.
type execStepView struct {
	name        string
	stepType    string
	tty         bool
	interactive bool
	hasRetry    bool
	hasTimeout  bool
	output      string
}

// ValidateExecTasks validates exec steps in a task list (custom command steps).
// Any `type: exec` step must be the final step and must not set fields that
// are meaningless once the process is replaced (tty, interactive, retry,
// timeout, output).
func ValidateExecTasks(tasks Tasks) error {
	views := make([]execStepView, 0, len(tasks))
	for i := range tasks {
		task := &tasks[i]
		views = append(views, execStepView{
			name:        task.Name,
			stepType:    task.Type,
			tty:         task.Tty,
			interactive: task.Interactive,
			hasRetry:    task.Retry != nil,
			hasTimeout:  task.Timeout != time.Duration(0),
			output:      task.Output,
		})
	}
	return validateExecSteps(views)
}

// ValidateExecWorkflowSteps validates exec steps in a workflow step list.
// See ValidateExecTasks for the rules.
func ValidateExecWorkflowSteps(steps []WorkflowStep) error {
	views := make([]execStepView, 0, len(steps))
	for i := range steps {
		step := &steps[i]
		views = append(views, execStepView{
			name:        step.Name,
			stepType:    step.Type,
			tty:         step.Tty,
			interactive: step.Interactive,
			hasRetry:    step.Retry != nil,
			hasTimeout:  step.Timeout != "",
			output:      step.Output,
		})
	}
	return validateExecSteps(views)
}

// containerStepType is the step type that supports background services in v1.
const containerStepType = "container"

// ValidateWorkflowSteps validates top-level workflow steps and any control-step children.
func ValidateWorkflowSteps(steps []WorkflowStep) error {
	if err := ValidateExecWorkflowSteps(steps); err != nil {
		return err
	}
	if err := validateBackgroundSteps(steps); err != nil {
		return err
	}
	return validateControlSteps(steps, false, "")
}

// validateBackgroundSteps enforces the background-step rules over the top-level
// workflow step list:
//   - `background: true` is supported only for `type: container` in v1, and a
//     background step must be non-interactive.
//   - a background step's name must be unique while it is live: the registry keys
//     handles by name, so a duplicate would overwrite (and leak) the earlier one.
//     Reuse is allowed only after the earlier step is `cancel`led.
//   - a `wait`/`cancel` step's `for:` targets must reference a background step
//     declared earlier in the workflow (and not already cancelled).
func validateBackgroundSteps(steps []WorkflowStep) error {
	live := make(map[string]bool)
	for i := range steps {
		step := &steps[i]
		stepType := effectiveWorkflowStepType(step.Type)
		if step.BackgroundAsync {
			if err := validateBackgroundStep(step, i, stepType); err != nil {
				return err
			}
			name := workflowStepName(step, i)
			if live[name] {
				return fmt.Errorf("%w: %s reuses the name of a still-running background step %q; background step names must be unique while live (cancel the earlier one first)",
					ErrWorkflowControlStepInvalid, workflowStepLabel(step, i), name)
			}
			live[name] = true
		}
		switch stepType {
		case TaskTypeWait, TaskTypeCancel:
			if err := validateBackgroundTargets(step, i, stepType, live); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateBackgroundStep checks a single `background: true` step.
func validateBackgroundStep(step *WorkflowStep, index int, stepType string) error {
	if stepType != containerStepType {
		return fmt.Errorf("%w: %s sets `background: true` but is type %q; v1 supports background only for `type: container` (shell/atmos background is planned)",
			ErrWorkflowControlStepInvalid, workflowStepLabel(step, index), stepType)
	}
	if step.Tty || step.Interactive {
		return fmt.Errorf("%w: background %s cannot set tty or interactive", ErrWorkflowControlStepInvalid, workflowStepLabel(step, index))
	}
	return nil
}

// validateBackgroundTargets checks a wait/cancel step's `for:` references and, for
// cancel, retires the targets so they cannot be cancelled or waited again.
func validateBackgroundTargets(step *WorkflowStep, index int, stepType string, live map[string]bool) error {
	if len(step.For) == 0 {
		return fmt.Errorf("%w: %s %s requires `for:` naming the background step(s) to %s",
			ErrWorkflowControlStepInvalid, stepType, workflowStepLabel(step, index), stepType)
	}
	for _, target := range step.For {
		if !live[target] {
			return fmt.Errorf("%w: %s %s references unknown or already-stopped background step %q in `for:`",
				ErrWorkflowControlStepInvalid, stepType, workflowStepLabel(step, index), target)
		}
	}
	if stepType == TaskTypeCancel {
		for _, target := range step.For {
			delete(live, target)
		}
	}
	return nil
}

// validateExecSteps enforces the exec step rules over the projected views.
func validateExecSteps(views []execStepView) error {
	for i := range views {
		view := &views[i]
		if view.stepType != TaskTypeExec {
			continue
		}
		if i != len(views)-1 {
			return fmt.Errorf("%w: step %s", ErrExecStepNotLast, execStepLabel(view, i))
		}
		if field := execStepInvalidField(view); field != "" {
			return fmt.Errorf("%w: step %s sets %q", ErrExecStepInvalidField, execStepLabel(view, i), field)
		}
	}
	return nil
}

// execStepInvalidField returns the name of the first field set on an exec
// step that is incompatible with process replacement, or "" if none.
func execStepInvalidField(view *execStepView) string {
	switch {
	case view.tty:
		return "tty"
	case view.interactive:
		return "interactive"
	case view.hasRetry:
		return "retry"
	case view.hasTimeout:
		return "timeout"
	case view.output != "":
		return "output"
	}
	return ""
}

// execStepLabel returns a human-friendly identifier for a step in error messages.
func execStepLabel(view *execStepView, index int) string {
	if view.name != "" {
		return fmt.Sprintf("%q (index %d)", view.name, index)
	}
	return fmt.Sprintf("%d", index)
}

func validateControlSteps(steps []WorkflowStep, inConcurrentGroup bool, parent string) error {
	if inConcurrentGroup {
		names, err := collectWorkflowStepNames(steps, parent)
		if err != nil {
			return err
		}
		if err := validateNeedsGraph(steps, names, parent); err != nil {
			return err
		}
	}
	return validateControlStepList(steps, inConcurrentGroup, parent)
}

func collectWorkflowStepNames(steps []WorkflowStep, parent string) (map[string]int, error) {
	names := make(map[string]int, len(steps))
	for i := range steps {
		name := workflowStepName(&steps[i], i)
		if prev, ok := names[name]; ok {
			return nil, fmt.Errorf("%w: duplicate step name %q in %s (indexes %d and %d)", ErrWorkflowControlStepInvalid, name, workflowScope(parent), prev, i)
		}
		names[name] = i
	}
	return names, nil
}

func validateControlStepList(steps []WorkflowStep, inConcurrentGroup bool, parent string) error {
	for i := range steps {
		step := &steps[i]
		stepType := effectiveWorkflowStepType(step.Type)
		if inConcurrentGroup {
			if err := validateConcurrentChild(step, i, parent); err != nil {
				return err
			}
			continue
		}
		if len(step.Needs) > 0 {
			return fmt.Errorf("%w: %s sets needs outside a concurrent control step", ErrWorkflowControlStepInvalid, workflowStepLabel(step, i))
		}
		if !isWorkflowControlStep(stepType) {
			continue
		}
		if err := validateControlStep(step, i); err != nil {
			return err
		}
		if err := validateControlSteps(step.Steps, true, workflowStepName(step, i)); err != nil {
			return err
		}
	}
	return nil
}

func validateControlStep(step *WorkflowStep, index int) error {
	label := workflowStepLabel(step, index)
	stepType := effectiveWorkflowStepType(step.Type)
	if len(step.Steps) == 0 {
		return fmt.Errorf("%w: %s requires at least one nested step", ErrWorkflowControlStepInvalid, label)
	}
	if step.MaxConcurrency < 0 {
		return fmt.Errorf("%w: %s sets negative max_concurrency", ErrWorkflowControlStepInvalid, label)
	}
	if err := validateControlFail(step, label); err != nil {
		return err
	}
	if err := validateParallelOutput(step, label); err != nil {
		return err
	}
	if stepType == TaskTypeMatrix {
		return validateControlMatrix(step, label)
	}
	return nil
}

func validateControlFail(step *WorkflowStep, label string) error {
	if step.Fail == nil {
		return nil
	}
	switch step.Fail.Mode {
	case "", "wait_all", "fail_fast", "best_effort":
	default:
		return fmt.Errorf("%w: %s sets unsupported fail.mode %q", ErrWorkflowControlStepInvalid, label, step.Fail.Mode)
	}
	if step.Fail.MaxFailures < 0 {
		return fmt.Errorf("%w: %s sets negative fail.max_failures", ErrWorkflowControlStepInvalid, label)
	}
	return nil
}

func validateControlMatrix(step *WorkflowStep, label string) error {
	if len(step.Matrix) == 0 {
		return fmt.Errorf("%w: %s requires at least one matrix axis", ErrWorkflowControlStepInvalid, label)
	}
	for axis, values := range step.Matrix {
		if strings.TrimSpace(axis) == "" {
			return fmt.Errorf("%w: %s contains an empty matrix axis name", ErrWorkflowControlStepInvalid, label)
		}
		if len(values) == 0 {
			return fmt.Errorf("%w: %s matrix axis %q must contain at least one value", ErrWorkflowControlStepInvalid, label, axis)
		}
	}
	return nil
}

func validateParallelOutput(step *WorkflowStep, label string) error {
	mode := strings.TrimSpace(step.Output)
	if step.ParallelOutput != nil {
		mode = strings.TrimSpace(step.ParallelOutput.Mode)
		switch strings.TrimSpace(step.ParallelOutput.Order) {
		case "", "completion", "definition":
		default:
			return fmt.Errorf("%w: %s sets unsupported output.order %q", ErrWorkflowControlStepInvalid, label, step.ParallelOutput.Order)
		}
	}
	switch mode {
	case "", "grouped", "prefixed", "none":
		return nil
	default:
		return fmt.Errorf("%w: %s sets unsupported output mode %q", ErrWorkflowControlStepInvalid, label, mode)
	}
}

func validateConcurrentChild(step *WorkflowStep, index int, parent string) error {
	label := workflowStepLabel(step, index)
	stepType := effectiveWorkflowStepType(step.Type)
	switch stepType {
	case TaskTypeAtmos, TaskTypeShell, "sleep":
	default:
		return fmt.Errorf("%w: %s cannot run inside concurrent step %q; allowed types are atmos, shell, and sleep", ErrWorkflowControlStepInvalid, label, parent)
	}
	if step.Tty || step.Interactive {
		return fmt.Errorf("%w: %s cannot set tty or interactive inside concurrent step %q", ErrWorkflowControlStepInvalid, label, parent)
	}
	switch strings.TrimSpace(step.Output) {
	case "", "log", "none":
	default:
		return fmt.Errorf("%w: %s cannot set child output mode %q inside concurrent step %q", ErrWorkflowControlStepInvalid, label, step.Output, parent)
	}
	if len(step.Steps) > 0 {
		return fmt.Errorf("%w: %s cannot declare nested steps inside concurrent step %q", ErrWorkflowControlStepInvalid, label, parent)
	}
	return nil
}

func validateNeedsGraph(steps []WorkflowStep, names map[string]int, parent string) error {
	graph := make(map[string][]string, len(steps))
	for i := range steps {
		name := workflowStepName(&steps[i], i)
		for _, need := range steps[i].Needs {
			if _, ok := names[need]; !ok {
				return fmt.Errorf("%w: step %q in %s needs unknown step %q", ErrWorkflowControlStepInvalid, name, workflowScope(parent), need)
			}
			graph[name] = append(graph[name], need)
		}
	}

	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("%w: cyclic needs dependency involving step %q in %s", ErrWorkflowControlStepInvalid, name, workflowScope(parent))
		}
		visiting[name] = true
		for _, dep := range graph[name] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}
	for name := range names {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func effectiveWorkflowStepType(stepType string) string {
	if strings.TrimSpace(stepType) == "" {
		return TaskTypeAtmos
	}
	return strings.TrimSpace(stepType)
}

func isWorkflowControlStep(stepType string) bool {
	return stepType == TaskTypeParallel || stepType == TaskTypeMatrix
}

func workflowStepName(step *WorkflowStep, index int) string {
	if strings.TrimSpace(step.Name) != "" {
		return step.Name
	}
	return fmt.Sprintf("step%d", index+1)
}

func workflowStepLabel(step *WorkflowStep, index int) string {
	if strings.TrimSpace(step.Name) != "" {
		return fmt.Sprintf("step %q", step.Name)
	}
	return fmt.Sprintf("step at index %d", index)
}

func workflowScope(parent string) string {
	if parent == "" {
		return "workflow"
	}
	return fmt.Sprintf("control step %q", parent)
}
