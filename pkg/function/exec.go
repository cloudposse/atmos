package function

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ShellExecutor defines the interface for executing shell commands.
// This allows for dependency injection and testing.
type ShellExecutor interface {
	// Execute runs a shell command and returns the output.
	Execute(ctx context.Context, command, workingDir string, env []string) (string, error)
}

// ExecFunction implements the !exec YAML function.
// It executes a shell command and returns the output.
type ExecFunction struct {
	BaseFunction
	executor ShellExecutor
}

// NewExecFunction creates a new ExecFunction with the given shell executor.
func NewExecFunction(executor ShellExecutor) *ExecFunction {
	defer perf.Track(nil, "function.NewExecFunction")()

	return &ExecFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "exec",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
		executor: executor,
	}
}

// Execute processes the !exec function.
// Syntax: !exec command [args...]
// Returns the command output, parsed as JSON if valid, otherwise as a string.
func (f *ExecFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.ExecFunction.Execute")()

	args = strings.TrimSpace(args)
	if args == "" {
		return nil, fmt.Errorf("%w: !exec requires a command", ErrInvalidArguments)
	}

	if f.executor == nil {
		return nil, fmt.Errorf("%w: shell executor not configured", ErrExecutionFailed)
	}

	// Determine working directory.
	workingDir := "."
	if execCtx != nil && execCtx.WorkingDir != "" {
		workingDir = execCtx.WorkingDir
	}

	// Build environment variables.
	var env []string
	if execCtx != nil {
		for k, v := range execCtx.Env {
			env = append(env, k+"="+v)
		}
	}

	// Execute the command.
	output, err := f.executor.Execute(ctx, args, workingDir, env)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrExecutionFailed, err)
	}

	// Try to parse output as JSON, fall back to string.
	var decoded any
	if err := json.Unmarshal([]byte(output), &decoded); err != nil {
		return output, nil
	}

	return decoded, nil
}
