//go:build !windows

package asciicast

import (
	"context"
	"os/exec"

	"github.com/creack/pty"
)

func startSessionShell(ctx context.Context, opts *SessionOptions) (*sessionProcess, error) {
	cmd := exec.CommandContext(ctx, sessionShell(opts.Shell)) //nolint:gosec // The shell is user/config supplied for an explicit interactive cast session.
	cmd.Dir = opts.Dir
	cmd.Env = sessionEnvironment(opts.Env)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: safePTYSize(opts.Width), Rows: safePTYSize(opts.Height)})
	if err != nil {
		return nil, err
	}
	return &sessionProcess{
		input:  ptmx,
		output: ptmx,
		close:  ptmx.Close,
		kill: func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		},
		wait: newSessionProcessWait(cmd.Wait),
	}, nil
}
