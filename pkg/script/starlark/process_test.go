package starlark

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestProcessDirectoryAndEnvironmentIsolation(t *testing.T) {
	t.Parallel()
	before, err := os.Getwd()
	require.NoError(t, err)
	dir := t.TempDir()
	baseEnv := []string{"BASE=original", "EXTRA=inherited", "BASE=step"}
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		name := spec.Args[0]
		assert.Equal(t, filepath.Join(dir, name), spec.Dir)
		assert.ElementsMatch(t, []string{"BASE=" + name, "EXTRA=inherited"}, spec.Env)
		_, _ = io.WriteString(spec.Streams.Stdout, name)
		_, _ = io.WriteString(spec.Streams.Stderr, "log")
		return process.Result{}
	})
	result, err := New(WithProcessRunner(runner)).Execute(context.Background(), script.Spec{
		WorkingDirectory: dir, ProcessEnv: baseEnv, Source: `
def run(name):
    result = exec.run(["tool", name], working_directory=name, env={"BASE": name})
    return {"stdout": result.stdout, "stderr": result.stderr, "code": result.exit_code}
output = steps.parallel(tasks=[steps.task(name=n, function=run, args=[n]) for n in ["a", "b"]])`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[{"stdout":"a","stderr":"log","code":0},{"stdout":"b","stderr":"log","code":0}]`, result.Value)
	assert.Equal(t, []string{"BASE=original", "EXTRA=inherited", "BASE=step"}, baseEnv)
	after, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestProcessValidation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`exec.run([])`, `exec.run([1])`, `exec.run("bad")`, `exec.run(["tool"], env={1:"a"})`, `exec.run(["tool"], env={"a":1})`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			_, err := runSource(t, source, WithProcessRunner(runner))
			require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
		})
	}
}

func TestCancellationStopsRunningAndQueuedTasks(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := NewMockRunner(gomock.NewController(t))
	var calls atomic.Int32
	started := make(chan struct{})
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		calls.Add(1)
		close(started)
		<-ctx.Done()
		return process.Result{Err: ctx.Err()}
	})
	done := make(chan error, 1)
	go func() {
		_, err := New(WithProcessRunner(runner)).Execute(ctx, script.Spec{Source: `
def branch():
    exec.run(["block"])
steps.parallel(functions=[branch, branch, branch], max_concurrency=1)`})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("branch did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not join")
	}
	assert.Equal(t, int32(1), calls.Load())
}

func TestCanceledInvocationHasNoEffects(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := NewMockRunner(gomock.NewController(t))
	_, err := New(WithProcessRunner(runner)).Execute(ctx, script.Spec{Source: `exec.run(["never"])`})
	require.ErrorIs(t, err, context.Canceled)
}

func TestProcessFailureWithoutCause(t *testing.T) {
	t.Parallel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{ExitCode: 7})
	_, err := runSource(t, `exec.run(["fail"])`, WithProcessRunner(runner))
	require.ErrorIs(t, err, errUtils.ErrProcessWaitFailed)
	assert.Contains(t, err.Error(), "code 7")
}
