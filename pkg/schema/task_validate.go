package schema

import (
	"errors"
	"fmt"
	"time"
)

// Sentinel errors for exec step validation.
var (
	// ErrExecStepNotLast is returned when a step of type exec is not the final step.
	ErrExecStepNotLast = errors.New("exec step must be the last step (the process is replaced; later steps would never run)")
	// ErrExecStepInvalidField is returned when an exec step sets a field that
	// is meaningless after process replacement.
	ErrExecStepInvalidField = errors.New("field is not supported on exec steps (the process is replaced)")
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
