//go:build !windows

package process

import (
	"context"
	"os/exec"
)

// newShellCommand builds the system-shell invocation for a session command
// string: `sh -c <command>`.
func newShellCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", command)
}
