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

	cockroach "github.com/cockroachdb/errors"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
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
	Policy           automation.ExecutionPolicy
	// Label names the command in failures; the executable (argv[0]) when empty. Nested Atmos
	// calls set it to the readable `atmos <args>` form, because argv[0] is the binary's path.
	Label string
	// NestedAtmos marks argv as an Atmos invocation. Its exit code errUtils.ExitCodePolicyDenied
	// then means a policy denied the command, and a failure is reported as such.
	NestedAtmos bool
}

// ProcessOutput contains captured output, independently of whether it was streamed.
type ProcessOutput struct {
	Stdout, Stderr string
	ExitCode       int
	// PolicyDenied reports that a nested Atmos call exited with errUtils.ExitCodePolicyDenied.
	PolicyDenied bool
}

// RunProcess applies capture, streaming, cancellation and exit-status policy.
// Writers are supplied by the host/binding, retaining its masking and task attribution.
func RunProcess(ctx context.Context, runner process.Runner, call *ProcessCall) (ProcessOutput, error) {
	defer perf.Track(nil, "script.RunProcess")()
	return runProcessWithPolicy(ctx, runner, call)
}

func runProcessOnce(ctx context.Context, runner process.Runner, call *ProcessCall) (ProcessOutput, bool, error) {
	if err := ctx.Err(); err != nil {
		return ProcessOutput{}, false, err
	}
	if len(call.Argv) == 0 {
		return ProcessOutput{}, false, serviceFailure(errUtils.ErrScriptInvalidArgument, nil, "argv must not be empty")
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
	output.PolicyDenied = call.NestedAtmos && normalNonzeroExit(&result) && result.ExitCode == errUtils.ExitCodePolicyDenied
	check := call.Check && (!call.AllowPlanChanges || result.ExitCode != 2)
	return output, normalNonzeroExit(&result), checkProcessResult(ctx, call, &result, &output, check)
}

// stderrShown reports whether stderr reached the user as it was produced. A failure then must not
// repeat its tail, which would print the same lines twice.
func (c *ProcessCall) stderrShown() bool {
	return c.Stream && c.Stderr != nil
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

func checkProcessResult(ctx context.Context, call *ProcessCall, result *process.Result, output *ProcessOutput, check bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if result.Success() || (!check && normalNonzeroExit(result)) {
		return nil
	}
	return processFailure(call, result, output)
}

// processFailure describes a failed subprocess: a start failure never reads as an exit code.
// The tail of stderr is attached only when the user has not already seen it, that is when the
// output was captured instead of streamed. A policy denial of a nested Atmos call is named as
// such and keeps its exit code, so the parent process exits with errUtils.ExitCodePolicyDenied too.
func processFailure(call *ProcessCall, result *process.Result, output *ProcessOutput) error {
	command := call.Argv[0]
	if call.Label != "" {
		command = call.Label
	}
	cause := result.Err
	if cause == nil {
		cause = errUtils.ErrProcessWaitFailed
	}
	if !result.Started && result.ExitCode < 0 {
		reason := strings.TrimPrefix(cause.Error(), errUtils.ErrProcessStartFailed.Error()+": ")
		return serviceFailure(errUtils.ErrScriptProcessFailed, cause, "failed to start %s: %s", command, reason)
	}
	err := serviceFailure(errUtils.ErrScriptProcessFailed, cause, "%s", exitMessage(command, result, output.PolicyDenied))
	if tail := lastLines(output.Stderr, maxStderrLines); tail != "" && !call.stderrShown() {
		err = withProcessDetail(err, fmt.Sprintf("Last lines of stderr from `%s`:\n%s", command, "```text\n"+tail+"\n```"))
	}
	if output.PolicyDenied {
		return errUtils.WithExitCode(err, errUtils.ExitCodePolicyDenied)
	}
	return err
}

// exitMessage reads "<command> exited with code N", or names the policy denial.
func exitMessage(command string, result *process.Result, denied bool) string {
	message := fmt.Sprintf("%s exited with code %d", command, result.ExitCode)
	if denied {
		message = deniedMessage(command, result.ExitCode)
	}
	if result.Signaled && result.Signal != "" {
		message += fmt.Sprintf(" (signal: %s)", result.Signal)
	}
	return message
}

// deniedMessage reports a nested Atmos command that a policy denied.
func deniedMessage(command string, exitCode int) string {
	return fmt.Sprintf("a policy denied %s (exit code %d)", command, exitCode)
}

// AtmosLabel names a nested Atmos invocation by its readable command line.
func AtmosLabel(args []string) string {
	defer perf.Track(nil, "script.AtmosLabel")()

	return strings.TrimSpace("atmos " + strings.Join(args, " "))
}

// NestedAtmosFailure describes a nested Atmos call, started outside RunProcess, that exited with a
// non-zero code. The message names the command and the code and, for errUtils.ExitCodePolicyDenied,
// says a policy denied it; that code is also the exit code of the returned error. The cause stays
// reachable through errors.Is and errors.As, so existing exit-code lookups keep working. The
// caller's output was streamed to the user, so no stderr tail is attached.
func NestedAtmosFailure(args []string, exitCode int, cause error) error {
	defer perf.Track(nil, "script.NestedAtmosFailure")()

	label := AtmosLabel(args)
	message := fmt.Sprintf("%s exited with code %d", label, exitCode)
	denied := exitCode == errUtils.ExitCodePolicyDenied
	if denied {
		message = deniedMessage(label, exitCode)
	}
	err := cockroach.Mark(serviceFailure(errUtils.ErrScriptProcessFailed, cause, "%s", message), errUtils.ErrNestedAtmosFailed)
	if denied {
		err = errUtils.WithExitCode(err, errUtils.ExitCodePolicyDenied)
	}
	return err
}

// MarkNestedAtmosFailure marks the error of a nested Atmos command, started by a workflow or custom
// command step through the shell executor, whose child exited with a non-zero code. The child reports
// its own failure, including its CI annotation, so the parent must not annotate it again. The error is
// otherwise unchanged: its message, exit code, and errors.As lookups stay the same. Silent exits,
// launch failures, and cancellations carry no child report and are returned as they are.
func MarkNestedAtmosFailure(err error) error {
	defer perf.Track(nil, "script.MarkNestedAtmosFailure")()

	var exitErr errUtils.ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Silent || exitErr.Code <= 0 {
		return err
	}
	return cockroach.Mark(err, errUtils.ErrNestedAtmosFailed)
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
