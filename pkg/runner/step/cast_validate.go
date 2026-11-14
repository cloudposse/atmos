package step

import (
	"fmt"
	"regexp"
	"time"

	"github.com/cloudposse/atmos/pkg/duration"
	"github.com/cloudposse/atmos/pkg/schema"
)

func validateCastStepsMode(step *schema.WorkflowStep) error {
	if len(step.Steps) == 0 {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastStepRequiresSteps, step.Name)
	}
	if step.Jitter < 0 || step.Jitter > 1 {
		return fmt.Errorf("%w: %v", ErrInvalidSimulateJitter, step.Jitter)
	}
	for i := range step.Steps {
		if err := validateCastStepChild(step, &step.Steps[i], i); err != nil {
			return err
		}
	}
	return nil
}

func validateCastSessionMode(step *schema.WorkflowStep) error {
	if len(step.Steps) == 0 {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrCastSessionRequiresActions, step.Name)
	}
	for i := range step.Steps {
		if err := validateCastSessionAction(step, &step.Steps[i]); err != nil {
			return fmt.Errorf("cast session action %d: %w", i+1, err)
		}
	}
	return nil
}

func validateCastSessionAction(castStep, action *schema.WorkflowStep) error {
	switch action.Type {
	case castPlaybackStepType:
		return validateCastPlaybackGroup(castStep, action, true)
	case "write":
		return validateWriteAction(action)
	case "key":
		return validateKeyAction(action)
	case "pause":
		return validatePauseAction(action)
	case "wait":
		return validateWaitAction(action)
	case "hide", "show":
		return nil
	case schema.TaskTypeScreenshot:
		return validateCastScreenshotAction(action)
	case schema.TaskTypeSimulate:
		child := *action
		applyCastSimulateDefaults(castStep, &child)
		return validateCastSimulateStep(&child)
	default:
		return validateCastSessionExecAction(action)
	}
}

// validateCastSessionExecAction validates a session action that isn't one of
// the PTY-driving verbs or "simulate": it must name a registered step type
// (e.g. "shell", "atmos", "script"), and that handler's own Validate must
// accept it -- mirroring how mode: steps validates its children.
func validateCastSessionExecAction(action *schema.WorkflowStep) error {
	actionType := action.Type
	if actionType == "" {
		actionType = schema.TaskTypeShell
	}
	handler, ok := Get(actionType)
	if !ok {
		return fmt.Errorf(wrappedQuotedErrorFormat, ErrUnsupportedSessionAction, action.Type)
	}
	return handler.Validate(action)
}

func validateWriteAction(action *schema.WorkflowStep) error {
	if action.Text == "" {
		return ErrWriteActionRequiresText
	}
	if _, err := parseDurationDefault(action.Rate, 0); action.Rate != "" && err != nil {
		return err
	}
	return nil
}

func validateKeyAction(action *schema.WorkflowStep) error {
	if action.Key == "" {
		return ErrKeyActionRequiresKey
	}
	if action.Interval == "" {
		return nil
	}
	if _, err := duration.ParseNonNegative(action.Interval); err != nil {
		return fmt.Errorf("key interval: %w", err)
	}
	return nil
}

func validatePauseAction(action *schema.WorkflowStep) error {
	if action.Duration == "" {
		return ErrPauseActionRequiresDuration
	}
	if _, err := duration.ParseNonNegative(action.Duration); err != nil {
		return fmt.Errorf("pause duration: %w", err)
	}
	return nil
}

func validateWaitAction(action *schema.WorkflowStep) error {
	hasText := action.Text != ""
	hasRegex := action.Regex != ""
	if hasText == hasRegex {
		return ErrWaitActionRequiresTextOrRegex
	}
	if hasRegex {
		if _, err := regexp.Compile(action.Regex); err != nil {
			return fmt.Errorf("wait regex: %w", err)
		}
	}
	if action.Timeout == "" {
		return nil
	}
	if _, err := duration.ParseNonNegative(action.Timeout); err != nil {
		return fmt.Errorf("wait timeout: %w", err)
	}
	return nil
}

func parseDurationDefault(value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := duration.ParseNonNegative(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", value, err)
	}
	return parsed, nil
}

func validateCastStepChild(step, child *schema.WorkflowStep, index int) error {
	switch child.Type {
	case schema.TaskTypeSimulate:
		clone := *child
		applyCastSimulateDefaults(step, &clone)
		if err := validateCastSimulateStep(&clone); err != nil {
			return fmt.Errorf("cast simulate step %d: %w", index+1, err)
		}
	case schema.TaskTypeScreenshot:
		if child.Path == "" {
			return fmt.Errorf("cast screenshot step %d: %w", index+1, ErrScreenshotActionRequiresPath)
		}
	case castPlaybackStepType:
		if err := validateCastPlaybackGroup(step, child, false); err != nil {
			return err
		}
	case castSessionStepType:
		if err := validateCastSessionMode(child); err != nil {
			return fmt.Errorf("cast session step %d: %w", index+1, err)
		}
	}
	return nil
}

func validateCastScreenshotAction(action *schema.WorkflowStep) error {
	if action.Path == "" {
		return ErrScreenshotActionRequiresPath
	}
	return nil
}
