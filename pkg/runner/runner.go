package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/shell"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Sentinel errors for task execution.
var (
	// ErrUnknownTaskType is returned when a task has an unknown type.
	ErrUnknownTaskType = errors.New("unknown task type")
)

// CommandRunner abstracts the execution of shell and atmos commands.
// This interface enables testing task execution without spawning real processes.
// It is designed to be compatible with pkg/workflow.CommandRunner.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=runner.go -destination=mock_runner_test.go -package=runner
type CommandRunner interface {
	// RunShell executes a shell command with the given parameters.
	// Parameters:
	//   - ctx: Context for cancellation and timeout
	//   - command: The shell command to execute
	//   - name: A name for the command (for logging/identification)
	//   - dir: Working directory for the command
	//   - env: Environment variables for the command
	//   - dryRun: If true, don't actually execute the command
	// Returns an error if the command fails.
	RunShell(ctx context.Context, command, name, dir string, env []string, dryRun bool) error

	// RunAtmos executes an atmos command with the given parameters.
	// Returns an error if the command fails.
	RunAtmos(ctx context.Context, params *AtmosExecParams) error
}

// AtmosExecParams holds parameters for executing an atmos command.
type AtmosExecParams struct {
	// AtmosConfig is the atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// Args are command arguments (e.g., ["terraform", "plan", "vpc"]).
	Args []string
	// Dir is the working directory for the command.
	Dir string
	// Env are environment variables for the command.
	Env []string
	// DryRun if true, don't actually execute the command.
	DryRun bool
}

// Options configures task execution.
type Options struct {
	// DryRun if true, commands are not actually executed.
	DryRun bool
	// Env are additional environment variables for the command.
	Env []string
	// Dir is the default working directory. Overridden by Task.WorkingDirectory.
	Dir string
	// AtmosConfig is the atmos configuration.
	AtmosConfig *schema.AtmosConfiguration
	// Stack overrides the task's stack setting (for command-line override).
	Stack string
}

// Run executes a single task with the given options.
// It handles timeout enforcement via context and delegates to the appropriate
// executor method based on task type.
func Run(ctx context.Context, task *Task, runner CommandRunner, opts Options) error {
	defer perf.Track(opts.AtmosConfig, "runner.Run")()

	// Apply timeout if specified.
	if task.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.Timeout)
		defer cancel()
	}

	// Determine working directory (task overrides options).
	dir := opts.Dir
	if task.WorkingDirectory != "" {
		dir = task.WorkingDirectory
	}
	if dir == "" {
		dir = "."
	}

	// Determine task type.
	taskType := task.Type
	if taskType == "" {
		taskType = schema.TaskTypeShell
	}

	switch taskType {
	case schema.TaskTypeShell:
		return runner.RunShell(ctx, task.Command, task.Name, dir, opts.Env, opts.DryRun)
	case schema.TaskTypeAtmos:
		return runAtmosTask(ctx, task, runner, opts, dir)
	default:
		return fmt.Errorf("%w: %s", ErrUnknownTaskType, taskType)
	}
}

// runAtmosTask executes an atmos-type task.
func runAtmosTask(ctx context.Context, task *Task, runner CommandRunner, opts Options, dir string) error {
	// Parse command using shell.Fields for proper quote handling.
	args, parseErr := shell.Fields(task.Command, nil)
	if parseErr != nil {
		// Fall back to simple split if shell parsing fails.
		args = strings.Fields(task.Command)
	}

	// Determine final stack (opts.Stack overrides task.Stack).
	finalStack := task.Stack
	if opts.Stack != "" {
		finalStack = opts.Stack
	}

	// Add stack argument if specified.
	if finalStack != "" {
		args = appendStackArg(args, finalStack)
	}

	params := &AtmosExecParams{
		AtmosConfig: opts.AtmosConfig,
		Args:        args,
		Dir:         dir,
		Env:         opts.Env,
		DryRun:      opts.DryRun,
	}
	return runner.RunAtmos(ctx, params)
}

// appendStackArg adds -s <stack> to the args, handling -- separator correctly.
func appendStackArg(args []string, stack string) []string {
	// Find -- separator if present.
	for i, arg := range args {
		if arg != "--" {
			continue
		}
		// Insert before -- separator.
		result := make([]string, 0, len(args)+2)
		result = append(result, args[:i]...)
		result = append(result, "-s", stack)
		result = append(result, args[i:]...)
		return result
	}
	// No -- separator, append at end.
	return append(args, "-s", stack)
}

// RunAll executes multiple tasks sequentially.
// It stops at the first error and returns it.
func RunAll(ctx context.Context, tasks Tasks, runner CommandRunner, opts Options) error {
	defer perf.Track(opts.AtmosConfig, "runner.RunAll")()

	for i, task := range tasks {
		if err := Run(ctx, &task, runner, opts); err != nil {
			return fmt.Errorf("task %d (%s) failed: %w", i, taskName(&task, i), err)
		}
	}
	return nil
}

// taskName returns a display name for the task.
func taskName(task *Task, index int) string {
	if task.Name != "" {
		return task.Name
	}
	return fmt.Sprintf("step%d", index+1)
}
