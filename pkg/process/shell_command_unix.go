//go:build !windows

package process

import (
	"context"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
)

// NewShellCommand builds the system-shell invocation for a session command
// string: `sh -c <command>`.
func NewShellCommand(ctx context.Context, command string) *exec.Cmd {
	defer perf.Track(nil, "process.NewShellCommand")()

	return exec.CommandContext(ctx, "sh", "-c", command)
}

// newShellCommand builds the system-shell invocation for a session command
// string: `sh -c <command>`.
func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	return NewShellCommand(ctx, command)
}
