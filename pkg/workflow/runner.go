package workflow

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// ShellExecutor is a function type for executing shell commands.
// This type allows injecting the actual shell execution function.
type ShellExecutor func(command, name, dir string, env []string, dryRun bool) error

// AtmosExecutor is a function type for executing atmos commands.
// This type allows injecting the actual atmos execution function.
type AtmosExecutor func(params *AtmosExecParams) error

// DefaultCommandRunner is the default implementation of CommandRunner
// that delegates to the provided shell and atmos execution functions.
// This is designed for dependency injection - the actual execution functions
// must be provided by the caller (typically from internal/exec).
type DefaultCommandRunner struct {
	shellExecutor ShellExecutor
	atmosExecutor AtmosExecutor
}

// NewDefaultCommandRunner creates a new DefaultCommandRunner with the given executors.
// Both executors must be provided - nil executors will cause panics at runtime.
func NewDefaultCommandRunner(shellExec ShellExecutor, atmosExec AtmosExecutor) *DefaultCommandRunner {
	return &DefaultCommandRunner{
		shellExecutor: shellExec,
		atmosExecutor: atmosExec,
	}
}

// RunShell executes a shell command using the configured shell executor.
func (r *DefaultCommandRunner) RunShell(command, name, dir string, env []string, dryRun bool) error {
	defer perf.Track(nil, "workflow.DefaultCommandRunner.RunShell")()

	if r.shellExecutor == nil {
		panic("shellExecutor is nil - DefaultCommandRunner was not properly initialized")
	}
	return r.shellExecutor(command, name, dir, env, dryRun)
}

// RunAtmos executes an atmos command using the configured atmos executor.
func (r *DefaultCommandRunner) RunAtmos(params *AtmosExecParams) error {
	defer perf.Track(params.AtmosConfig, "workflow.DefaultCommandRunner.RunAtmos")()

	if r.atmosExecutor == nil {
		panic("atmosExecutor is nil - DefaultCommandRunner was not properly initialized")
	}
	return r.atmosExecutor(params)
}
