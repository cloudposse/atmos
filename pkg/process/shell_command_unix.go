//go:build !windows

package process

import (
	"context"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
)

// NewShellCommand builds the system-shell invocation for a session command
// string: `sh -c <command>`. Callers must pass only command strings authored in
// trusted configuration (e.g. atmos.yaml) -- command is interpolated into a real
// shell invocation with no escaping of its own.
func NewShellCommand(ctx context.Context, command string) *exec.Cmd {
	defer perf.Track(nil, "process.NewShellCommand")()

	return exec.CommandContext(ctx, "sh", "-c", command)
}

// applyWindowsCmdExeQuoting is a no-op outside Windows: POSIX's exec.Cmd
// passes Args to execve verbatim (no shell-style re-parsing), so the
// cmd.exe-specific quoting problem this works around on Windows (see the
// windows-build variant) doesn't exist here.
func applyWindowsCmdExeQuoting(cmd *exec.Cmd, program string, args []string) {}
