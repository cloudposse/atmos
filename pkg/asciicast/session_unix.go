//go:build !windows

package asciicast

import (
	"context"
	"os/exec"

	"github.com/creack/pty"
)

func startSessionShell(ctx context.Context, opts *SessionOptions) (*sessionProcess, error) {
	launch, err := sessionShellCommand(opts.Shell, opts.ShellInit)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, launch.Executable, launch.Args...) //nolint:gosec // The shell is user/config supplied for an explicit interactive cast session.
	cmd.Dir = opts.Dir
	cmd.Env = sessionEnvironment(launch.environment(opts.Env))
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: safePTYSize(opts.Width), Rows: safePTYSize(opts.Height)})
	if err != nil {
		launch.release()
		return nil, err
	}
	return &sessionProcess{
		input:  ptmx,
		output: ptmx,
		close: func() error {
			err := ptmx.Close()
			launch.release()
			return err
		},
		kill: func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		},
		wait: newSessionProcessWait(cmd.Wait),
	}, nil
}
