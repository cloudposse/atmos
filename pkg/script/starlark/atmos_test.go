package starlark

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestAtmosStructuredArgumentsAndInvocationDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "atmos binary")
	for _, tc := range []struct {
		name, source string
		args         []string
	}{
		{"toolchain install", `atmos.toolchain("install", "jq@1.7.1", flags={"force": True}, args=["--literal=two words"])`, []string{"toolchain", "install", "jq@1.7.1", "--force", "--literal=two words"}},
		{"toolchain list", `atmos.toolchain("list")`, []string{"toolchain", "list"}},
		{"custom command", `atmos.run(["casts", "validate", "demo", "fixtures", "starlark", "release-plan"])`, []string{"casts", "validate", "demo", "fixtures", "starlark", "release-plan"}},
		{"terraform", `atmos.terraform("plan", component="vpc", stack="dev", flags={"detailed-exitcode": True, "-var": ["message=hello world", "literal=$(touch nope)"], "-parallelism": 2})`, []string{"terraform", "plan", "vpc", "--stack=dev", "-parallelism=2", "-var=message=hello world", "-var=literal=$(touch nope)", "-detailed-exitcode"}},
		{"helm", `atmos.helm("apply", component="api", stack="dev", flags={"wait": False}, args=["--timeout=5m"])`, []string{"helm", "apply", "api", "--stack=dev", "--wait=false", "--timeout=5m"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				assert.Equal(t, binary, spec.Command)
				assert.Equal(t, tc.args, spec.Args)
				assert.Equal(t, dir, spec.Dir)
				assert.Equal(t, []string{"BASE=inherited"}, spec.Env)
				_, _ = io.WriteString(spec.Streams.Stdout, "data")
				_, _ = io.WriteString(spec.Streams.Stderr, "progress")
				return process.Result{}
			})
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "component"), 0o700))
			var stdout, stderr bytes.Buffer
			result, err := New(WithAtmosCommands(testAtmosCatalog()), WithProcessRunner(runner), WithAtmosExecutable(func() (string, error) { return binary, nil })).Execute(context.Background(), script.Spec{
				WorkingDirectory: filepath.Join(dir, "component"), AtmosWorkingDirectory: dir,
				ProcessEnv: []string{"BASE=inherited"}, Stdout: &stdout, Stderr: &stderr,
				Source: "r = " + tc.source + "\noutput = [r.stdout, r.stderr, r.exit_code]",
			})
			require.NoError(t, err)
			assert.JSONEq(t, `["data","progress",0]`, result.Value)
			assert.Equal(t, "data", stdout.String())
			assert.Equal(t, "progress", stderr.String())
		})
	}
}

func TestAtmosCaptureAndOverrides(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		assert.Equal(t, filepath.Join(dir, "other"), spec.Dir)
		assert.Equal(t, "child", envpkg.SliceToMap(spec.Env)["BASE"])
		_, _ = io.WriteString(spec.Streams.Stdout, "captured")
		_, _ = io.WriteString(spec.Streams.Stderr, "diagnostic")
		return process.Result{Started: true, ExitCode: 7, Err: errUtils.ErrProcessWaitFailed}
	})
	var stdout, stderr bytes.Buffer
	result, err := New(WithAtmosCommands(testAtmosCatalog()), WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		AtmosWorkingDirectory: dir, ProcessEnv: []string{"BASE=parent"}, Stdout: &stdout, Stderr: &stderr,
		Source: `r = atmos.run(["custom"], working_directory="other", env={"BASE": "child"}, check=False, output="capture")
output = [r.stdout, r.stderr, r.exit_code]`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `["captured","diagnostic",7]`, result.Value)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestAtmosExitSemantics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		result       process.Result
		wantError    bool
	}{
		{"plan changes", `atmos.terraform("plan", component="vpc", stack="dev", flags={"detailed-exitcode": True})`, process.Result{Started: true, ExitCode: 2, Err: errUtils.ErrProcessWaitFailed}, false},
		{"plan failure", `atmos.terraform("plan", component="vpc", stack="dev", flags={"detailed-exitcode": True})`, process.Result{Started: true, ExitCode: 1, Err: errUtils.ErrProcessWaitFailed}, true},
		{"ordinary plan", `atmos.terraform("plan", component="vpc", stack="dev")`, process.Result{Started: true, ExitCode: 2}, true},
		{"plan args", `atmos.terraform("plan", component="vpc", stack="dev", args=["-detailed-exitcode"])`, process.Result{Started: true, ExitCode: 2}, false},
		{"disabled detailed code", `atmos.terraform("plan", component="vpc", stack="dev", flags={"detailed-exitcode": True}, args=["-detailed-exitcode=false"])`, process.Result{Started: true, ExitCode: 2}, true},
		{"helm failure", `atmos.helm("apply", component="api", stack="dev")`, process.Result{Started: true, ExitCode: 2}, true},
		{"toolchain failure", `atmos.toolchain("install", "jq@1.7.1")`, process.Result{Started: true, ExitCode: 1}, true},
		{"generic command", `atmos.run(["custom"])`, process.Result{Started: true, ExitCode: 2}, true},
		{"launch error", `atmos.run(["custom"], check=False)`, process.Result{ExitCode: 2, Err: errUtils.ErrProcessStartFailed}, true},
		{"signal", `atmos.run(["custom"], check=False)`, process.Result{Started: true, ExitCode: 2, Signaled: true}, true},
		{"transport error", `atmos.terraform("plan", component="vpc", stack="dev", flags={"detailed-exitcode": True})`, process.Result{Started: true, ExitCode: 2, Err: io.ErrClosedPipe}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(tc.result)
			result, err := runSource(t, "r = "+tc.source+"\noutput = r.exit_code", WithProcessRunner(runner))
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "2", result.Value)
			}
		})
	}
}

func TestAtmosValidation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`atmos.toolchain("", "jq")`, `atmos.toolchain("--install")`, `atmos.toolchain("install", "--flag")`, `atmos.toolchain("install", args=[1])`, `atmos.toolchain("install", flags={1: True})`,
		`atmos.run([])`, `atmos.run("version")`, `atmos.run([1])`,
		`atmos.terraform("plan", component="vpc")`, `atmos.helm("apply", component="--flag", stack="dev")`,
		`atmos.terraform("plan", component="vpc", stack="", flags={})`,
		`atmos.helm("apply", component="api", stack="dev", flags={"stack": "prod"})`,
		`atmos.helm("apply", component="api", stack="dev", flags={"bad key": True})`,
		`atmos.helm("apply", component="api", stack="dev", flags={1: True})`,
		`atmos.terraform("plan", component="api", stack="dev", flags={"detailed-exitcode": True, "-detailed-exitcode": False})`,
		`atmos.helm("apply", component="api", stack="dev", flags={"wait": {}})`,
		`atmos.helm("apply", component="api", stack="dev", args=[1])`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, source, WithProcessRunner(NewMockRunner(gomock.NewController(t))))
			require.ErrorIs(t, err, errUtils.ErrStarlark)
		})
	}
	_, err := runSource(t, `atmos.run(["version"])`, WithAtmosExecutable(func() (string, error) { return "", errors.New("missing binary") }))
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.ErrorContains(t, err, "missing binary")
}

func TestAtmosParallelRetries(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	var attempts atomic.Int32
	joined := make(chan struct{})
	var running atomic.Int32
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(3).DoAndReturn(func(ctx context.Context, spec process.TaskSpec) process.Result {
		if running.Add(1) == 2 {
			close(joined)
		}
		select {
		case <-joined:
		case <-ctx.Done():
			return process.Result{Err: ctx.Err()}
		}
		name := spec.Args[2]
		if name == "api" && attempts.Add(1) == 1 {
			return process.Result{Started: true, ExitCode: 1, Err: errUtils.ErrProcessWaitFailed}
		}
		_, _ = io.WriteString(spec.Streams.Stdout, name)
		return process.Result{}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := New(WithAtmosCommands(testAtmosCatalog()), WithProcessRunner(runner)).Execute(ctx, script.Spec{Source: `
def plan(name):
    return atmos.terraform("plan", component=name, stack="dev", output="capture").stdout
output = steps.parallel(tasks=[steps.task(name=n, function=plan, args=[n], retry={"max_attempts": 2, "initial_delay": "1ms"}) for n in ["api", "worker"]], max_concurrency=2)
`})
	require.NoError(t, err)
	assert.JSONEq(t, `["api","worker"]`, result.Value)
	assert.Equal(t, int32(2), attempts.Load())
}

func TestAtmosCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ process.TaskSpec) process.Result {
		cancel()
		return process.Result{Started: true, ExitCode: 1, Canceled: true, Err: ctx.Err()}
	})
	_, err := New(WithAtmosCommands(testAtmosCatalog()), WithProcessRunner(runner)).Execute(ctx, script.Spec{Source: `atmos.run(["custom"], check=False)`})
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, strings.Contains(err.Error(), "code 1"))
}
