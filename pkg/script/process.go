package script

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
)

// ProcessCall describes a host command after language-specific argument conversion.
// Env is explicit; an empty environment never inherits the Atmos process environment.
type ProcessCall struct {
	Argv             []string
	Dir              string
	Env              []string
	Check, Stream    bool
	AllowPlanChanges bool
	Stdout, Stderr   io.Writer
}

// ProcessOutput contains captured output, independently of whether it was streamed.
type ProcessOutput struct {
	Stdout, Stderr string
	ExitCode       int
}

// RunProcess applies capture, streaming, cancellation and exit-status policy.
// Writers are supplied by the host/binding, retaining its masking and task attribution.
func RunProcess(ctx context.Context, runner process.Runner, call *ProcessCall) (ProcessOutput, error) {
	defer perf.Track(nil, "script.RunProcess")()
	if err := ctx.Err(); err != nil {
		return ProcessOutput{}, err
	}
	if len(call.Argv) == 0 {
		return ProcessOutput{}, serviceFailure(errUtils.ErrScriptInvalidArgument, nil, "argv must not be empty")
	}
	var stdout, stderr bytes.Buffer
	var out, diagnostic io.Writer = &stdout, &stderr
	if call.Stream {
		if call.Stdout != nil {
			out = io.MultiWriter(out, call.Stdout)
		}
		if call.Stderr != nil {
			diagnostic = io.MultiWriter(diagnostic, call.Stderr)
		}
	}
	result := runner.Run(ctx, process.TaskSpec{
		Command: call.Argv[0], Args: call.Argv[1:], Dir: call.Dir, Env: ProcessEnvironment(call.Env, nil),
		Streams: process.Streams{Stdout: out, Stderr: diagnostic},
	})
	output := ProcessOutput{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: result.ExitCode}
	check := call.Check && (!call.AllowPlanChanges || result.ExitCode != 2)
	return output, checkProcessResult(ctx, call.Argv[0], &result, output.Stderr, check)
}

// ProcessEnvironment copies inherited values and applies per-call overrides in
// stable order. Every duplicate inherited key is removed before its override.
func ProcessEnvironment(base []string, values map[string]string) []string {
	defer perf.Track(nil, "script.ProcessEnvironment")()

	env := append([]string{}, base...)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		// Inherited layers may contain the same key more than once. Remove every
		// occurrence so the per-call value wins for both lookup and subprocesses.
		env = slices.DeleteFunc(env, func(entry string) bool {
			return strings.HasPrefix(entry, key+"=")
		})
		env = append(env, key+"="+values[key])
	}
	return env
}

// Upper limit of captured stderr lines echoed into process failure explanations.
const maxStderrLines = 20

func checkProcessResult(ctx context.Context, command string, result *process.Result, stderr string, check bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if result.Success() || (!check && normalNonzeroExit(result)) {
		return nil
	}
	return processFailure(command, result, stderr)
}

// processFailure describes a failed subprocess: a start failure never reads as an exit code.
func processFailure(command string, result *process.Result, stderr string) error {
	cause := result.Err
	if cause == nil {
		cause = errUtils.ErrProcessWaitFailed
	}
	if !result.Started && result.ExitCode < 0 {
		reason := strings.TrimPrefix(cause.Error(), errUtils.ErrProcessStartFailed.Error()+": ")
		return serviceFailure(errUtils.ErrScriptProcessFailed, cause, "failed to start %s: %s", command, reason)
	}
	message := fmt.Sprintf("%s exited with code %d", command, result.ExitCode)
	if result.Signaled && result.Signal != "" {
		message += fmt.Sprintf(" (signal: %s)", result.Signal)
	}
	err := serviceFailure(errUtils.ErrScriptProcessFailed, cause, "%s", message)
	if tail := lastLines(stderr, maxStderrLines); tail != "" {
		err = withProcessDetail(err, fmt.Sprintf("Last lines of stderr from `%s`:\n%s", command, "```text\n"+tail+"\n```"))
	}
	return err
}

// lastLines returns at most n trailing non-empty lines of text.
func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// normalNonzeroExit excludes failures to launch, cancellation, signals and I/O errors.
func normalNonzeroExit(result *process.Result) bool {
	return result.Started && result.ExitCode > 0 && !result.Canceled && !result.Signaled &&
		!errors.Is(result.Err, context.Canceled) && !errors.Is(result.Err, context.DeadlineExceeded) &&
		(result.Err == nil || errors.Is(result.Err, errUtils.ErrProcessWaitFailed))
}
