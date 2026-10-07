package starlark

import (
	"errors"
	"path/filepath"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/automation"
	"github.com/cloudposse/atmos/pkg/script"
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
	var timeout string
	var retryValue starlark.Value = starlark.None
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "argv", &argv, "working_directory?", &dir, "env?", &extraEnv,
		"output?", &opts.output, "check?", &opts.check, "timeout?", &timeout, "retry?", &retryValue); err != nil {
		return nil, err
	}
	policy, err := s.processPolicy(thread, timeout, retryValue)
	if err != nil {
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
	return s.runProcess(thread, &processCall{argv: command, dir: dir, env: env, check: opts.check, stream: stream, policy: policy})
}

type processCall struct {
	argv             []string
	dir              string
	env              []string
	check, stream    bool
	allowPlanChanges bool
	policy           automation.ExecutionPolicy
}

func (s *session) runProcess(thread *starlark.Thread, call *processCall) (starlark.Value, error) {
	result, err := script.RunProcess(threadContext(thread), s.engine.runner, &script.ProcessCall{
		Argv: call.argv, Dir: call.dir, Env: s.tools.Environment(call.env),
		Check: call.check, Stream: call.stream, AllowPlanChanges: call.allowPlanChanges,
		Stdout: s.writer(thread, stdoutStream), Stderr: s.writer(thread, stderrStream),
		Policy: call.policy,
	})
	if err != nil {
		if errors.Is(err, errUtils.ErrScriptProcessFailed) {
			return nil, failWith(errUtils.ErrStarlarkProcessFailed, err, "%s", err)
		}
		return nil, err
	}
	return newProcessResult(result), nil
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
	values := make(map[string]string)
	if extra != nil {
		for _, item := range extra.Items() {
			key, keyOK := starlark.AsString(item[0])
			value, valueOK := starlark.AsString(item[1])
			if !keyOK || !valueOK {
				return nil, invalidArg("process env must contain strings")
			}
			values[key] = value
		}
	}
	return script.ProcessEnvironment(base, values), nil
}
