package script

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
)

//go:generate go run go.uber.org/mock/mockgen -destination mock_process_test.go -package script github.com/cloudposse/atmos/pkg/process Runner

func TestProcessService(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                           string
		status                         process.Result
		check, stream, plan, wantError bool
	}{
		{name: "stream", status: process.Result{Started: true}, check: true, stream: true},
		{name: "capture", status: process.Result{Started: true}, check: true},
		{name: "unchecked exit", status: process.Result{Started: true, ExitCode: 7, Err: errUtils.ErrProcessWaitFailed}},
		{name: "checked exit", status: process.Result{Started: true, ExitCode: 7, Err: errUtils.ErrProcessWaitFailed}, check: true, wantError: true},
		{name: "plan changes", status: process.Result{Started: true, ExitCode: 2, Err: errUtils.ErrProcessWaitFailed}, check: true, plan: true},
		{name: "ordinary exit two", status: process.Result{Started: true, ExitCode: 2, Err: errUtils.ErrProcessWaitFailed}, check: true, wantError: true},
		{name: "launch", status: process.Result{ExitCode: -1, Err: errUtils.ErrProcessStartFailed}, wantError: true},
		{name: "signal", status: process.Result{Started: true, ExitCode: 2, Signaled: true}, check: true, plan: true, wantError: true},
		{name: "io", status: process.Result{Started: true, ExitCode: 2, Err: io.ErrClosedPipe}, plan: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				assert.Equal(t, "tool", spec.Command)
				assert.Equal(t, []string{"one argument"}, spec.Args)
				require.NotNil(t, spec.Env, "empty environment must not inherit the host environment")
				_, _ = io.WriteString(spec.Streams.Stdout, "output")
				_, _ = io.WriteString(spec.Streams.Stderr, "diagnostic")
				return tc.status
			})
			var stdout, stderr bytes.Buffer
			output, err := RunProcess(t.Context(), runner, &ProcessCall{Argv: []string{"tool", "one argument"}, Check: tc.check, Stream: tc.stream, AllowPlanChanges: tc.plan, Stdout: &stdout, Stderr: &stderr})
			assert.Equal(t, ProcessOutput{Stdout: "output", Stderr: "diagnostic", ExitCode: tc.status.ExitCode}, output)
			if tc.wantError {
				require.ErrorIs(t, err, errUtils.ErrScriptProcessFailed)
				if tc.status.Err != nil {
					require.ErrorIs(t, err, tc.status.Err)
				}
			} else {
				require.NoError(t, err)
			}
			if tc.stream {
				assert.Equal(t, "output", stdout.String())
				assert.Equal(t, "diagnostic", stderr.String())
			} else {
				assert.Empty(t, stdout.String())
				assert.Empty(t, stderr.String())
			}
		})
	}
}

func TestProcessCancellationAndValidation(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := RunProcess(ctx, runner, &ProcessCall{Argv: []string{"never"}})
	require.ErrorIs(t, err, context.Canceled)
	_, err = RunProcess(t.Context(), runner, &ProcessCall{})
	require.ErrorIs(t, err, errUtils.ErrScriptInvalidArgument)
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, process.TaskSpec) process.Result { cancel(); return process.Result{Started: true} })
	_, err = RunProcess(ctx, runner, &ProcessCall{Argv: []string{"tool"}})
	require.ErrorIs(t, err, context.Canceled)
}

func TestProcessEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	base := []string{"KEEP=yes", "VALUE=old", "VALUE=duplicate"}
	overrides := map[string]string{"VALUE": "new", "ANOTHER": "value"}
	env := ProcessEnvironment(base, overrides)
	assert.Equal(t, []string{"KEEP=yes", "ANOTHER=value", "VALUE=new"}, env)
	env[0] = "KEEP=changed"
	assert.Equal(t, "KEEP=yes", base[0])
	base[1] = "VALUE=changed"
	overrides["VALUE"] = "changed"
	assert.Equal(t, "VALUE=new", env[2])
}

func TestToolPinsAndEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	calls := 0
	dirs := []string{filepath.Join(t.TempDir(), "tools")}
	originalDir := dirs[0]
	tools := NewTools(func(_ context.Context, pins map[string]string) ([]string, error) {
		calls++
		assert.Equal(t, map[string]string{"jq": "1"}, pins)
		return dirs, nil
	})
	require.NoError(t, tools.Install(t.Context(), "jq", "1"))
	dirs[0] = "mutated"
	require.NoError(t, tools.Install(t.Context(), "jq", "1"))
	require.ErrorIs(t, tools.Install(t.Context(), "jq", "2"), errUtils.ErrScriptInvalidArgument)
	assert.Equal(t, 1, calls)
	base := []string{"PATH=original"}
	env := tools.Environment(base)
	assert.Equal(t, originalDir+string(os.PathListSeparator)+"original", envpkg.SliceToMap(env)["PATH"])
	env[0] = "changed"
	assert.Equal(t, []string{"PATH=original"}, base)
	assert.Equal(t, originalDir+string(os.PathListSeparator)+"original", envpkg.SliceToMap(tools.Environment(base))["PATH"])
	assert.Equal(t, base, NewTools(nil).Environment(base), "pins must not leak between invocations")
	require.ErrorIs(t, NewTools(nil).Install(t.Context(), "jq", "1"), errUtils.ErrScript)
}

func TestToolCancellationDoesNotCommitPins(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	tools := NewTools(func(context.Context, map[string]string) ([]string, error) {
		calls++
		if calls == 1 {
			cancel()
		}
		return []string{"tools"}, nil
	})
	require.ErrorIs(t, tools.Install(ctx, "jq", "1"), context.Canceled)
	assert.Empty(t, tools.Environment(nil))
	require.NoError(t, tools.Install(t.Context(), "jq", "2"))
	assert.Equal(t, 2, calls)
}

func TestToolCanceledWaiter(t *testing.T) {
	t.Parallel()
	started, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tools := NewTools(func(ctx context.Context, _ map[string]string) ([]string, error) {
		close(started)
		select {
		case <-release:
			return []string{"tools"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	done := make(chan error, 1)
	go func() { done <- tools.Install(ctx, "jq", "1") }()
	<-started
	waiter, stop := context.WithCancel(t.Context())
	stop()
	require.ErrorIs(t, tools.Install(waiter, "jq", "1"), context.Canceled)
	assert.Empty(t, tools.Environment(nil), "readers see only completed pins")
	close(release)
	require.NoError(t, <-done)
}

func TestDiagnosticMetadataAndIsolation(t *testing.T) {
	t.Parallel()
	base := NewDiagnostic("base").With(func(b *errUtils.ErrorBuilder) { b.WithHint("base hint").WithExitCode(2) })
	child := base.With(func(b *errUtils.ErrorBuilder) { b.WithHint("child hint") })
	outer := NewDiagnostic("outer").WithCause(child).With(func(b *errUtils.ErrorBuilder) { b.WithExitCode(7).WithContext("stack", "dev") })
	causes := outer.Unwrap()
	causes[0] = errors.New("mutated")
	assert.Equal(t, "outer: base", outer.Error())
	builder := errUtils.Build(errUtils.ErrScript)
	EnrichDiagnostics(builder, errors.Join(outer, child))
	err := builder.Err()
	assert.ElementsMatch(t, []string{"base hint", "child hint"}, cockroach.GetAllHints(err))
	assert.Equal(t, 7, errUtils.GetExitCode(err))
	separate := errUtils.Build(errUtils.ErrScript)
	EnrichDiagnostics(separate, base)
	assert.Equal(t, []string{"base hint"}, cockroach.GetAllHints(separate.Err()))
}

func TestInvocationDefaultsAndWorkingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	spec := Spec{WorkingDirectory: dir, SourcePath: filepath.Join(dir, "script.star")}
	require.NoError(t, PrepareSpec(&spec))
	assert.Equal(t, "<script>", spec.Name)
	assert.Equal(t, io.Discard, spec.Stdout)
	assert.Equal(t, io.Discard, spec.Stderr)
	assert.Equal(t, spec.SourcePath, spec.File.Path)
	require.NoError(t, ValidateWorkingDirectory(dir))
	require.ErrorIs(t, ValidateWorkingDirectory(filepath.Join(dir, "missing")), os.ErrNotExist)
	require.NoError(t, os.WriteFile(spec.SourcePath, []byte("output=1"), 0o600))
	require.ErrorIs(t, ValidateWorkingDirectory(spec.SourcePath), errUtils.ErrScript)
}

func runFailing(t *testing.T, call *ProcessCall, code int) (ProcessOutput, error) {
	t.Helper()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		_, _ = io.WriteString(spec.Streams.Stderr, "boom: nested failure\n")
		return process.Result{Started: true, ExitCode: code, Err: errUtils.ErrProcessWaitFailed}
	})
	return RunProcess(context.Background(), runner, call)
}

func TestProcessFailureStderrTail(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		stream   bool
		stderr   io.Writer
		wantTail bool
	}{
		{"captured output is repeated", false, nil, true},
		{"streamed output reached the user", true, &bytes.Buffer{}, false},
		{"streaming without a writer reached nobody", true, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runFailing(t, &ProcessCall{Argv: []string{"tool"}, Check: true, Stream: tc.stream, Stderr: tc.stderr}, 1)
			require.ErrorIs(t, err, errUtils.ErrScriptProcessFailed)
			joined := strings.Join(cockroach.GetAllDetails(err), "\n")
			if tc.wantTail {
				assert.Contains(t, joined, "boom: nested failure")
				return
			}
			assert.NotContains(t, joined, "boom: nested failure")
		})
	}
}

func TestProcessPolicyDenial(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		call        ProcessCall
		code        int
		wantDenied  bool
		wantMessage string
		wantExit    int
	}{
		{"nested atmos denied", ProcessCall{Argv: []string{"/bin/atmos", "terraform", "destroy"}, Label: "atmos terraform destroy", NestedAtmos: true, Check: true}, 77, true, "a policy denied atmos terraform destroy (exit code 77)", 77},
		{"nested atmos ordinary failure", ProcessCall{Argv: []string{"/bin/atmos", "version"}, Label: "atmos version", NestedAtmos: true, Check: true}, 1, false, "atmos version exited with code 1", 1},
		{"other program with the same code", ProcessCall{Argv: []string{"tool"}, Check: true}, 77, false, "tool exited with code 77", 1},
		{"unchecked denial is reported on the output", ProcessCall{Argv: []string{"/bin/atmos"}, NestedAtmos: true}, 77, true, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output, err := runFailing(t, &tc.call, tc.code)
			assert.Equal(t, tc.wantDenied, output.PolicyDenied)
			assert.Equal(t, tc.code, output.ExitCode)
			if tc.wantMessage == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantMessage, err.Error())
			assert.Equal(t, tc.wantExit, errUtils.GetExitCode(err))
		})
	}
}

func TestNestedAtmosFailure(t *testing.T) {
	t.Parallel()
	cause := &exec.ExitError{}
	denied := NestedAtmosFailure([]string{"terraform", "destroy", "vpc", "-s", "dev"}, errUtils.ExitCodePolicyDenied, cause)
	assert.Equal(t, "a policy denied atmos terraform destroy vpc -s dev (exit code 77)", denied.Error())
	assert.Equal(t, errUtils.ExitCodePolicyDenied, errUtils.GetExitCode(denied))
	assert.ErrorIs(t, denied, errUtils.ErrScriptProcessFailed)
	var found *exec.ExitError
	assert.True(t, errors.As(denied, &found), "the cause stays reachable")

	ordinary := NestedAtmosFailure([]string{"terraform", "plan"}, 2, cause)
	assert.Equal(t, "atmos terraform plan exited with code 2", ordinary.Error())
	assert.Empty(t, cockroach.GetAllDetails(ordinary), "streamed output is not repeated")
}
