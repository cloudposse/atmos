package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgexec "github.com/cloudposse/atmos/pkg/exec"
	"github.com/cloudposse/atmos/pkg/perf"
)

// stderrTailLimit bounds the in-memory stderr tail kept for error
// classification. Stderr itself is streamed to RunOptions.Stderr (a masked
// writer in production); the tail is never embedded in error chains.
const stderrTailLimit = 4096

// RunOptions configures a single git subprocess invocation.
type RunOptions struct {
	// Dir is the working directory for the subprocess.
	Dir string
	// Env is the full subprocess environment; nil inherits the process env.
	Env []string
	// Stderr receives the subprocess stderr stream as it is produced.
	// Production callers pass a masked writer; nil discards.
	Stderr io.Writer
}

// RunResult is the outcome of a git subprocess invocation.
type RunResult struct {
	// Stdout is the captured standard output.
	Stdout string
	// ExitCode is the subprocess exit code (0 on success).
	ExitCode int
	// StderrTail holds the last bytes of stderr for error classification only.
	// It must never be embedded in error messages (it may contain secrets and
	// bypasses the writer-level masking pipeline).
	StderrTail string
}

// Runner executes git commands. The production implementation shells out;
// tests substitute a fake to assert command construction.
type Runner interface {
	Run(ctx context.Context, command string, args []string, opts RunOptions) (RunResult, error)
}

// ExecRunner is the production Runner using pkg/exec.CommandExecutor.
type ExecRunner struct {
	executor pkgexec.CommandExecutor
}

// NewExecRunner returns the production Runner.
func NewExecRunner() *ExecRunner {
	return NewExecRunnerWithExecutor(pkgexec.Default())
}

// NewExecRunnerWithExecutor returns a Runner backed by the provided command executor.
func NewExecRunnerWithExecutor(executor pkgexec.CommandExecutor) *ExecRunner {
	if executor == nil {
		executor = pkgexec.Default()
	}
	return &ExecRunner{executor: executor}
}

// Run executes the command and returns the captured result. A non-zero exit
// returns the result alongside an error wrapping ErrGitCommandExited; other
// failures (binary missing, context canceled) wrap ErrGitCommandFailed.
func (r *ExecRunner) Run(ctx context.Context, command string, args []string, opts RunOptions) (RunResult, error) {
	defer perf.Track(nil, "git.ExecRunner.Run")()

	var stdout bytes.Buffer
	tail := &tailBuffer{limit: stderrTailLimit}

	cmd := r.executor.CommandContext(ctx, command, args...)
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env
	cmd.Stdout = &stdout
	if opts.Stderr != nil {
		cmd.Stderr = io.MultiWriter(opts.Stderr, tail)
	} else {
		cmd.Stderr = tail
	}

	err := cmd.Run()
	result := RunResult{Stdout: stdout.String(), StderrTail: tail.String()}
	if err == nil {
		return result, nil
	}

	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("%w: %s %s (exit %d)", errUtils.ErrGitCommandExited, command, firstArg(args), result.ExitCode)
	}

	return result, fmt.Errorf("%w: %s: %w", errUtils.ErrGitCommandFailed, command, err)
}

// firstArg names the git subcommand for error context without echoing
// full argument lists (which may include URIs).
func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	for _, a := range args {
		if len(a) > 1 && a[0] == '-' {
			continue
		}
		return a
	}
	return args[0]
}

// tailBuffer keeps the last `limit` bytes written to it.
type tailBuffer struct {
	limit int
	buf   []byte
}

// Write appends p, trimming the front to stay within the limit.
func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.limit {
		t.buf = t.buf[len(t.buf)-t.limit:]
	}
	return len(p), nil
}

// String returns the buffered tail.
func (t *tailBuffer) String() string {
	return string(t.buf)
}
