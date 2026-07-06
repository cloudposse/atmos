package asciicast

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
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
		Env:     envMap(env),
	})
	if err != nil {
		return nil, err
	}

	//nolint:gosec // The command is caller-provided argv for recording, not shell input.
	cmd := exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
	cmd.Dir = opts.Dir
	cmd.Env = env
	cmd.Stdout = &streamWriter{rec: rec, stream: "o"}
	cmd.Stderr = &streamWriter{rec: rec, stream: "e"}

	runErr := cmd.Run()
	closeErr := rec.Close()
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if closeErr != nil {
			return nil, errors.Join(closeErr, runErr)
		}
		return &ExecResult{ExitCode: exitErr.ExitCode()}, nil
	}
	if joined := errors.Join(runErr, closeErr); joined != nil {
		return nil, joined
	}
	return &ExecResult{ExitCode: 0}, nil
}

func envMap(env []string) map[string]string {
	result := make(map[string]string, len(env))
	for _, pair := range env {
		for i := 0; i < len(pair); i++ {
			if pair[i] == '=' {
				result[pair[:i]] = pair[i+1:]
				break
			}
		}
	}
	return result
}
