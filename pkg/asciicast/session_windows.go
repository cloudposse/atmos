//go:build windows

package asciicast

import (
	"context"
	"io"
	"os/exec"
	"sync"
)

func startSessionShell(ctx context.Context, opts SessionOptions) (*sessionProcess, error) {
	cmd := exec.CommandContext(ctx, sessionShell(opts.Shell)) //nolint:gosec // The shell is user/config supplied for an explicit interactive cast session.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	outputReader, outputWriter := io.Pipe()
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = outputReader.Close()
		_ = outputWriter.Close()
		return nil, err
	}

	var once sync.Once
	closeAll := func() error {
		once.Do(func() {
			_ = stdin.Close()
			_ = outputReader.Close()
			_ = outputWriter.Close()
		})
		return nil
	}

	go func() {
		_ = cmd.Wait()
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
	}, nil
}
