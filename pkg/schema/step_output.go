package schema

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrStepInvalidOutputMode is returned when a step or workflow sets an output mode that does not
// exist.
var ErrStepInvalidOutputMode = errors.New("invalid step output mode")

// Output modes accepted by the command-running step types.
const (
	// StepOutputRaw passes output straight through to stdout and stderr.
	StepOutputRaw = "raw"
	// StepOutputLog streams output between step boundaries.
	StepOutputLog = "log"
	// StepOutputViewport shows output in a live, scrolling window.
	StepOutputViewport = "viewport"
	// StepOutputNone suppresses output.
	StepOutputNone = "none"
)

// StepOutputModes lists the output modes of command-running steps, in documentation order.
func StepOutputModes() []string {
	return []string{StepOutputRaw, StepOutputLog, StepOutputViewport, StepOutputNone}
}

// StepAcceptsOutputMode reports whether the step type renders command output through the shared
// output modes. Other types use their own `output` values (parallel, matrix, test, cast) or have
// no output to render.
func StepAcceptsOutputMode(stepType string) bool {
	switch strings.TrimSpace(stepType) {
	case "", TaskTypeShell, TaskTypeScript, TaskTypeAtmos, containerStepType:
		return true
	default:
		return false
	}
}

// ValidateOutputMode fails with ErrStepInvalidOutputMode when mode is set and is not one of
// StepOutputModes. An empty mode means "use the default". A templated mode is checked after
// rendering, so a value containing a template expression is accepted here.
func ValidateOutputMode(owner, mode string) error {
	if strings.Contains(mode, "{{") {
		return nil
	}
	return ValidateRenderedOutputMode(owner, mode)
}

// ValidateRenderedOutputMode is ValidateOutputMode for a mode that has already been rendered: it
// fails with ErrStepInvalidOutputMode for any value that is not one of StepOutputModes, including
// one that still contains a template expression. An empty mode means "use the default".
func ValidateRenderedOutputMode(owner, mode string) error {
	mode = strings.TrimSpace(mode)
	if mode == "" || slices.Contains(StepOutputModes(), mode) {
		return nil
	}
	return fmt.Errorf("%w: %s sets output %q; valid modes are %s",
		ErrStepInvalidOutputMode, owner, mode, strings.Join(StepOutputModes(), ", "))
}

// ValidateStepOutput checks the `output` mode of a single step of a type that accepts the shared
// output modes. Steps of other types are not checked.
func ValidateStepOutput(step *WorkflowStep) error {
	if !StepAcceptsOutputMode(step.Type) {
		return nil
	}
	owner := "a step"
	if step.Name != "" {
		owner = fmt.Sprintf("step %q", step.Name)
	}
	return ValidateOutputMode(owner, step.Output)
}

// validateStepOutputModes checks the output mode of every step, including the children of
// parallel and matrix steps.
func validateStepOutputModes(steps []WorkflowStep) error {
	for i := range steps {
		if err := ValidateStepOutput(&steps[i]); err != nil {
			return err
		}
		if err := validateStepOutputModes(steps[i].Steps); err != nil {
			return err
		}
	}
	return nil
}
