package schema

import "fmt"

// ValidateTestStep validates the test container without changing standalone control rules.
func ValidateTestStep(s *WorkflowStep) error {
	if len(s.Steps) == 0 {
		return fmt.Errorf("%w: test requires at least one step", ErrWorkflowControlStepInvalid)
	}
	if s.MaxConcurrency != 0 {
		return fmt.Errorf("%w: use parallel or matrix to configure test concurrency", ErrWorkflowControlStepInvalid)
	}
	if s.Output != "" && s.Output != "failures" && s.Output != "all" || s.ParallelOutput != nil {
		return fmt.Errorf("%w: test output must be failures or all", ErrWorkflowControlStepInvalid)
	}
	if err := validateControlFail(s, "test"); err != nil {
		return err
	}
	return validateTestChildren(s.Steps, false, s.Name)
}

// validateTestChildren checks dependency structure and validates every test child.
func validateTestChildren(steps []WorkflowStep, concurrent bool, parent string) error {
	names, err := collectWorkflowStepNames(steps, parent)
	if err != nil {
		return err
	}
	if err := validateNeedsGraph(steps, names, parent); err != nil {
		return err
	}
	for i := range steps {
		if err := validateTestChild(&steps[i], i, concurrent); err != nil {
			return err
		}
	}

	return nil
}

// validateTestChild enforces test restrictions and the leaf's execution contract.
func validateTestChild(s *WorkflowStep, index int, concurrent bool) error {
	if s.Tty || s.Interactive || s.BackgroundAsync {
		return fmt.Errorf("%w: test child %q must run in the foreground without a terminal", ErrWorkflowControlStepInvalid, s.Name)
	}
	switch s.Type {
	case TaskTypeTest, TaskTypeExec, "exit", TaskTypeCast, "session", "simulate", "emulator", TaskTypeWait, TaskTypeWaitAll, TaskTypeCancel:
		return fmt.Errorf("%w: unsupported test child type %q", ErrWorkflowControlStepInvalid, s.Type)
	}
	if len(s.Needs) > 0 && !concurrent {
		return fmt.Errorf("%w: needs requires a parallel or matrix parent", ErrWorkflowControlStepInvalid)
	}
	if s.Type != TaskTypeParallel && s.Type != TaskTypeMatrix {
		if len(s.Steps) > 0 {
			return fmt.Errorf("%w: test leaf %q cannot contain steps", ErrWorkflowControlStepInvalid, s.Name)
		}
		return ValidateExecWorkflowSteps([]WorkflowStep{*s})
	}
	return validateTestControl(s, index, concurrent)
}

// validateTestControl validates a concurrent group and its leaf children.
func validateTestControl(s *WorkflowStep, index int, concurrent bool) error {
	if concurrent {
		return fmt.Errorf("%w: parallel/matrix groups cannot be nested inside each other", ErrWorkflowControlStepInvalid)
	}
	if err := validateControlStep(s, index); err != nil {
		return err
	}
	return validateTestChildren(s.Steps, true, s.Name)
}
