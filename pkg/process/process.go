package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Runner executes a task and returns the observed process result.
type Runner interface {
	Run(ctx context.Context, spec TaskSpec) Result
}

// TaskSpec describes a subprocess invocation.
type TaskSpec struct {
	Command string
	Args    []string
	Dir     string
	Env     []string
	Streams Streams
	DryRun  bool
}

// Streams contains the standard streams passed to a subprocess.
type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Result contains the outcome of a subprocess invocation.
type Result struct {
	Command    string
	Args       []string
	ExitCode   int
	Err        error
	Started    bool
	Canceled   bool
	StartedAt  time.Time
	FinishedAt time.Time
}

// Success reports whether the subprocess completed with exit code 0.
func (r Result) Success() bool {
	return r.Err == nil && r.ExitCode == 0
}

// DefaultRunner executes tasks with os/exec.
type DefaultRunner struct{}

// NewDefaultRunner returns a Runner backed by os/exec.
func NewDefaultRunner() Runner {
	return DefaultRunner{}
}

// Run executes spec with context-aware cancellation and exit-code preservation.
func (r DefaultRunner) Run(ctx context.Context, spec TaskSpec) (result Result) {
	if ctx == nil {
		ctx = context.Background()
	}

	result = Result{
		Command:  spec.Command,
		Args:     append([]string{}, spec.Args...),
		ExitCode: 0,
	}
	defer func() {
		result.FinishedAt = time.Now()
	}()

	if spec.DryRun {
		return result
	}

	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	if spec.Env != nil {
		cmd.Env = append([]string{}, spec.Env...)
	}
	cmd.Stdin = spec.Streams.Stdin
	cmd.Stdout = writerOrDiscard(spec.Streams.Stdout)
	cmd.Stderr = writerOrDiscard(spec.Streams.Stderr)

	if err := cmd.Start(); err != nil {
		result.Err = fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessStartFailed, err)
		result.ExitCode = -1
		return result
	}
	result.Started = true
	result.StartedAt = time.Now()

	err := cmd.Wait()
	if err == nil {
		return result
	}

	result.Err = fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrProcessWaitFailed, err)
	result.ExitCode = exitCode(err)
	if ctxErr := ctx.Err(); ctxErr != nil {
		result.Canceled = true
		result.Err = errors.Join(result.Err, ctxErr)
	}
	return result
}

func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// OSStreams returns streams connected to the current process standard streams.
func OSStreams() Streams {
	return Streams{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}
