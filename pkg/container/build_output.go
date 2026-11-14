package container

import (
	"bytes"
	"context"
	"io"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/process"
)

// runBuildCommand streams diagnostics while retaining them for build errors.
func runBuildCommand(ctx context.Context, cmd *exec.Cmd, output io.Writer) ([]byte, error) {
	var captured bytes.Buffer
	var writer io.Writer = &captured
	if output != nil {
		writer = io.MultiWriter(&captured, output)
	}
	// The shared runner owns the process group and cleans up Buildx children on interruption.
	result := process.NewDefaultRunner().Run(ctx, process.TaskSpec{
		Command: cmd.Args[0], Path: cmd.Path, Args: cmd.Args[1:], Dir: cmd.Dir, Env: cmd.Env,
		Streams: process.Streams{Stdin: cmd.Stdin, Stdout: writer, Stderr: writer},
	})
	return captured.Bytes(), result.Err
}
