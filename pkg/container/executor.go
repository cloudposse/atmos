package container

import (
	"context"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandExecutor provides an abstraction for executing system commands.
// This interface allows for mocking command execution in unit tests.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=executor.go -destination=mock_executor_test.go -package=container
type CommandExecutor interface {
	// LookPath searches for an executable named file in PATH.
	LookPath(file string) (string, error)

	// CommandContext creates an exec.Cmd configured to run with the given context.
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

// defaultExecutor implements CommandExecutor using the standard library exec package.
type defaultExecutor struct{}

// newDefaultExecutor creates a new default command executor.
func newDefaultExecutor() CommandExecutor {
	return &defaultExecutor{}
}

// LookPath searches for an executable named file in PATH.
func (e *defaultExecutor) LookPath(file string) (string, error) {
	defer perf.Track(nil, "container.defaultExecutor.LookPath")()

	return exec.LookPath(file)
}

// CommandContext creates an exec.Cmd configured to run with the given context.
func (e *defaultExecutor) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	defer perf.Track(nil, "container.defaultExecutor.CommandContext")()

	return exec.CommandContext(ctx, name, args...)
}

// globalExecutor holds the package-level executor instance.
// Tests can override this to inject mock executors.
var globalExecutor CommandExecutor = newDefaultExecutor()

// setExecutor sets the global executor (for testing).
func setExecutor(executor CommandExecutor) {
	globalExecutor = executor
}

// resetExecutor resets the global executor to default (for testing).
func resetExecutor() {
	globalExecutor = newDefaultExecutor()
}
