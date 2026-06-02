// Package exec provides a small, mockable abstraction over the standard
// library os/exec package so that command execution can be substituted in
// unit tests without spawning real processes.
package exec

import (
	"context"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandExecutor provides an abstraction for executing system commands.
// This interface allows for mocking command execution in unit tests.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=exec.go -destination=mock_exec.go -package=exec
type CommandExecutor interface {
	// LookPath searches for an executable named file in PATH.
	LookPath(file string) (string, error)

	// CommandContext creates an exec.Cmd configured to run with the given context.
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

// defaultExecutor implements CommandExecutor using the standard library exec package.
type defaultExecutor struct{}

// Default returns a CommandExecutor backed by the standard library os/exec package.
func Default() CommandExecutor {
	defer perf.Track(nil, "exec.Default")()

	return &defaultExecutor{}
}

// LookPath searches for an executable named file in PATH.
func (e *defaultExecutor) LookPath(file string) (string, error) {
	defer perf.Track(nil, "exec.defaultExecutor.LookPath")()

	return exec.LookPath(file)
}

// CommandContext creates an exec.Cmd configured to run with the given context.
func (e *defaultExecutor) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	defer perf.Track(nil, "exec.defaultExecutor.CommandContext")()

	return exec.CommandContext(ctx, name, args...)
}
