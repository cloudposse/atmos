//go:build windows

package asciicast

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
)

func startSessionShell(ctx context.Context, opts *SessionOptions) (*sessionProcess, error) {
	launch, err := sessionShellCommand(opts.Shell, opts.ShellInit)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, launch.Executable, launch.Args...) //nolint:gosec // The shell is user/config supplied for an explicit interactive cast session.
	cmd.Dir = opts.Dir
	cmd.Env = sessionEnvironment(launch.environment(opts.Env))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		launch.release()
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}

	outputReader, outputWriter := io.Pipe()
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter

	if err := cmd.Start(); err != nil {
		launch.release()
		_ = stdin.Close()
		_ = outputReader.Close()
		_ = outputWriter.Close()
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}

	var once sync.Once
	waitCh := make(chan error, 1)
	closeAll := func() error {
		once.Do(func() {
			_ = stdin.Close()
			_ = outputReader.Close()
			_ = outputWriter.Close()
			launch.release()
		})
		return nil
	}

	go func() {
		waitCh <- cmd.Wait()
		_ = outputWriter.Close()
	}()

	return &sessionProcess{
		input:              stdin,
		output:             outputReader,
		closeInputOnFinish: true,
		close:              closeAll,
		kill: func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		},
		wait: newSessionProcessWait(func() error {
			return <-waitCh
		}),
	}, nil
}
