package errors

import (
	"fmt"
	"strings"
)

// ExecError represents a failure from executing an external command.
// This includes terraform, helmfile, shell commands, or any subprocess invocation.
type ExecError struct {
	Cmd      string   // Command name (e.g., "terraform", "helm", "sh").
	Args     []string // Command arguments.
	ExitCode int      // Non-zero exit code.
	Stderr   string   // Optional stderr output.
	cause    error    // Wrapped underlying error.
}

// Error returns the error message.
func (e *ExecError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	cmdStr := e.Cmd
	if len(e.Args) > 0 {
		cmdStr = cmdStr + " " + strings.Join(e.Args, " ")
	}
	return fmt.Sprintf("command %s exited with code %d", cmdStr, e.ExitCode)
}

// Unwrap returns the wrapped error.
func (e *ExecError) Unwrap() error {
	return e.cause
}

// NewExecError creates an error for external command execution failure.
func NewExecError(cmd string, args []string, exitCode int, cause error) *ExecError {
	return &ExecError{
		Cmd:      cmd,
		Args:     args,
		ExitCode: exitCode,
		cause:    cause,
	}
}

// WithStderr adds stderr output to the ExecError.
func (e *ExecError) WithStderr(stderr string) *ExecError {
	e.Stderr = stderr
	return e
}
