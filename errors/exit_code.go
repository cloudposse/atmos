package errors

import (
	"os/exec"

	"github.com/cockroachdb/errors"
)

const (
	// ExitCodeSIGINT is the POSIX exit code for SIGINT (Ctrl+C).
	// Calculated as 128 + signal_number, where SIGINT is signal 2.
	ExitCodeSIGINT = 130
)

// exitCoder wraps an error and specifies an exit code.
type exitCoder struct {
	cause error
	code  int
}

func (e *exitCoder) Error() string {
	return e.cause.Error()
}

func (e *exitCoder) Cause() error {
	return e.cause
}

func (e *exitCoder) Unwrap() error {
	return e.cause
}

// ExitCode returns the exit code.
func (e *exitCoder) ExitCode() int {
	return e.code
}

// WithExitCode attaches an exit code to an error.
// The exit code can be retrieved later using GetExitCode.
func WithExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &exitCoder{
		cause: err,
		code:  code,
	}
}

// GetExitCode extracts the exit code from an error chain.
// Returns 0 if err is nil, 1 by default, or the specified exit code.
//
// It checks for exit codes in this order:
//  1. WorkflowStepError from workflow orchestration.
//  2. ExecError from external command execution.
//  3. ExitCodeError from workflow/shell execution (legacy).
//  4. exitCoder attached via WithExitCode.
//  5. exec.ExitError from command execution.
//  6. Default to 1.
func GetExitCode(err error) int {
	if err == nil {
		return 0
	}

	// Check for WorkflowStepError (from workflow step execution).
	var workflowErr *WorkflowStepError
	if errors.As(err, &workflowErr) {
		return workflowErr.ExitCode
	}

	// Check for ExecError (from external command execution).
	var execErr *ExecError
	if errors.As(err, &execErr) {
		return execErr.ExitCode
	}

	// Check for ExitCodeError (from workflow/shell execution - legacy).
	var exitCodeErr ExitCodeError
	if errors.As(err, &exitCodeErr) {
		return exitCodeErr.Code
	}

	// Check for exitCoder.
	var ec *exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}

	// Check for exec.ExitError.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return 1 // default
}
