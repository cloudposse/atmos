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
	cmd := exec.CommandContext(ctx, sessionShell(opts.Shell)) //nolint:gosec // The shell is user/config supplied for an explicit interactive cast session.
	cmd.Dir = opts.Dir
	cmd.Env = sessionEnvironment(opts.Env)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
	}

	outputReader, outputWriter := io.Pipe()
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter

	if err := cmd.Start(); err != nil {
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
