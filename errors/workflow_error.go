package errors

import (
	"fmt"
)

// WorkflowStepError represents a failure from executing a workflow step.
// This is a higher-level orchestration error that wraps underlying command
// or shell execution failures with workflow-specific context.
type WorkflowStepError struct {
	Workflow string // Workflow name.
	Step     string // Step name or index.
	Command  string // Command that was attempted.
	ExitCode int    // Non-zero exit code from the failed command.
	cause    error  // Wrapped underlying error from command execution.
}

// Error returns the error message.
func (e *WorkflowStepError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	return "workflow step execution failed"
}

// Unwrap returns the wrapped error.
func (e *WorkflowStepError) Unwrap() error {
	return e.cause
}

// NewWorkflowStepError creates an error for workflow step execution failure.
func NewWorkflowStepError(workflow, step, command string, exitCode int, cause error) *WorkflowStepError {
	return &WorkflowStepError{
		Workflow: workflow,
		Step:     step,
		Command:  command,
		ExitCode: exitCode,
		cause:    cause,
	}
}

// WorkflowStepMessage returns the formatted workflow step error message.
// This is used by the formatter to generate user-facing error messages.
func (e *WorkflowStepError) WorkflowStepMessage() string {
	return fmt.Sprintf("workflow step execution failed with exit code %d", e.ExitCode)
}
