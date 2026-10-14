package starlark

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
)

func (s *session) exec(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return s.execScoped(thread, b, args, kwargs, processScope{dir: s.spec.WorkingDirectory, env: s.spec.ProcessEnv})
}

type processScope struct {
	dir string
	env []string
}

const (
	outputStreamMode  = "stream"
	outputCaptureMode = "capture"
)

// runOptions are the subprocess policies shared by exec.run, component.exec, and atmos.*.
type runOptions struct {
	check  bool
	output string
}

func newRunOptions() runOptions { return runOptions{check: true, output: outputStreamMode} }

// streaming reports whether subprocess output is forwarded to the step output.
func (o *runOptions) streaming() (bool, error) {
	switch o.output {
	case outputStreamMode, outputCaptureMode:
		return o.output == outputStreamMode, nil
	default:
		return false, invalidArg("output must be %q or %q, got %q", outputStreamMode, outputCaptureMode, o.output)
	}
}

func (s *session) execScoped(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple, scope processScope) (starlark.Value, error) {
	var argv starlark.Value
	var extraEnv *starlark.Dict
	dir := scope.dir
	opts := newRunOptions()
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "argv", &argv, "working_directory?", &dir, "env?", &extraEnv,
		"output?", &opts.output, "check?", &opts.check); err != nil {
		return nil, err
	}
	command, err := processArgv(argv)
	if err != nil {
		return nil, err
	}
	env, err := processEnv(scope.env, extraEnv)
	if err != nil {
		return nil, err
	}
	stream, err := opts.streaming()
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(scope.dir, dir)
	}
	return s.runProcess(thread, processCall{argv: command, dir: dir, env: env, check: opts.check, stream: stream})
}

type processCall struct {
	argv             []string
	dir              string
	env              []string
	check, stream    bool
	allowPlanChanges bool
}

func (s *session) runProcess(thread *starlark.Thread, call processCall) (starlark.Value, error) {
	var stdout, stderr bytes.Buffer
	var stdoutWriter, stderrWriter io.Writer = &stdout, &stderr
	if call.stream {
		stdoutWriter = io.MultiWriter(&stdout, s.writer(thread, stdoutStream))
		stderrWriter = io.MultiWriter(&stderr, s.writer(thread, stderrStream))
	}
	ctx := threadContext(thread)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for i := len(s.toolDirs) - 1; i >= 0; i-- {
		call.env = envpkg.UpdateEnvironmentPath(call.env, s.toolDirs[i])
	}
	result := s.engine.runner.Run(ctx, process.TaskSpec{
		Command: call.argv[0], Args: call.argv[1:], Dir: call.dir, Env: call.env,
		Streams: process.Streams{
			Stdout: stdoutWriter,
			Stderr: stderrWriter,
		},
	})
	check := call.check && (!call.allowPlanChanges || result.ExitCode != 2)
	if err := checkProcessResult(ctx, call.argv[0], &result, stderr.String(), check); err != nil {
		return nil, err
	}
	return starlarkstruct.FromStringDict(starlark.String("process_result"), starlark.StringDict{
		"stdout": starlark.String(stdout.String()), "stderr": starlark.String(stderr.String()), "exit_code": starlark.MakeInt(result.ExitCode),
	}), nil
}

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
		return failWith(errUtils.ErrStarlarkProcessFailed, cause, "failed to start %s: %s", command, reason)
	}
	message := fmt.Sprintf("%s exited with code %d", command, result.ExitCode)
	if result.Signaled && result.Signal != "" {
		message += fmt.Sprintf(" (signal: %s)", result.Signal)
	}
	err := failWith(errUtils.ErrStarlarkProcessFailed, cause, "%s", message)
	if tail := lastLines(stderr, maxStderrLines); tail != "" {
		err = withDetail(err, fmt.Sprintf("Last lines of stderr from `%s`:\n%s", command, fenced(tail)))
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

func processArgv(argv starlark.Value) ([]string, error) {
	values, err := sequence(argv)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, invalidArg("argv must not be empty")
	}
	command := make([]string, len(values))
	for i, value := range values {
		str, ok := starlark.AsString(value)
		if !ok {
			return nil, invalidArg("argv[%d] must be a string", i)
		}
		command[i] = str
	}
	return command, nil
}

func processEnv(base []string, extra *starlark.Dict) ([]string, error) {
	env := slices.Clone(base)
	if env == nil {
		env = []string{}
	}
	if extra == nil {
		return env, nil
	}
	values := make(map[string]string)
	for _, item := range extra.Items() {
		key, keyOK := starlark.AsString(item[0])
		value, valueOK := starlark.AsString(item[1])
		if !keyOK || !valueOK {
			return nil, invalidArg("process env must contain strings")
		}
		values[key] = value
	}
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
	return env, nil
}
