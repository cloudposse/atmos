package asciicast

import (
	"context"
	"errors"
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
)

// ErrMissingExecCommand indicates that ExecRecord was called without a command.
var ErrMissingExecCommand = errUtils.ErrMissingExecCommand

// ExecOptions configures a one-shot, non-interactive command recording.
// Unlike RunSession, no PTY is allocated: callers force color output through
// environment variables (e.g. ATMOS_FORCE_COLOR/CLICOLOR_FORCE) instead.
type ExecOptions struct {
	// Command is the argv to run; Command[0] is resolved via PATH.
	Command []string
	// Dir is the working directory for the command (empty = inherit).
	Dir string
	// Env is the full environment for the command (nil = inherit).
	Env []string
	// Path is the output .cast file path.
	Path string
	// Width and Height size the recorded terminal (0 = defaults).
	Width  int
	Height int
	// Title is an optional cast title.
	Title string
}

// ExecResult reports the outcome of a recorded command.
type ExecResult struct {
	// ExitCode is the command's exit status (0 on success).
	ExitCode int
}

// streamWriter forwards writes to the recorder as events on one stream.
// Write failures propagate so the recorded command aborts rather than
// silently producing an incomplete .cast file.
type streamWriter struct {
	rec    *Recorder
	stream string
}

//nolint:lintroller // Trivial io.Writer adapter on a hot output path - no perf tracking.
func (w *streamWriter) Write(p []byte) (int, error) {
	if err := w.rec.Event(w.stream, string(p)); err != nil {
		return 0, fmt.Errorf("record %s event: %w", w.stream, err)
	}
	return len(p), nil
}

// ExecRecord runs a command and records its stdout and stderr as asciicast
// output events. It returns the command's exit code in ExecResult; a non-zero
// exit is not an error (callers decide how to treat command failures).
func ExecRecord(ctx context.Context, opts *ExecOptions) (*ExecResult, error) {
	defer perf.Track(nil, "asciicast.ExecRecord")()

	if opts == nil || len(opts.Command) == 0 {
		return nil, ErrMissingExecCommand
	}
	env := opts.Env
	if env == nil {
		env = os.Environ()
	}
	rec, err := Start(&Options{
		Path:    opts.Path,
		Command: opts.Command,
		Title:   opts.Title,
		Width:   opts.Width,
		Height:  opts.Height,
		Env:     envpkg.SliceToMap(env),
	})
	if err != nil {
		return nil, err
	}

	result := process.NewDefaultRunner().Run(ctx, process.TaskSpec{
		Command: opts.Command[0],
		Args:    opts.Command[1:],
		Dir:     opts.Dir,
		Env:     env,
		Streams: process.Streams{
			Stdout: &streamWriter{rec: rec, stream: "o"},
			Stderr: &streamWriter{rec: rec, stream: "e"},
		},
	})
	closeErr := rec.Close()
	return execRecordResult(&result, closeErr)
}

func execRecordResult(result *process.Result, closeErr error) (*ExecResult, error) {
	if closeErr != nil {
		if result.Err != nil {
			return nil, errors.Join(closeErr, result.Err)
		}
		return nil, closeErr
	}
	if result.Err == nil {
		return &ExecResult{ExitCode: 0}, nil
	}
	if result.Canceled {
		return nil, result.Err
	}
	if result.ExitCode >= 0 {
		return &ExecResult{ExitCode: result.ExitCode}, nil
	}
	return nil, result.Err
}
