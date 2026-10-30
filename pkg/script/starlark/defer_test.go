package starlark

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/signals"
)

func TestDeferRunsLastInFirstOutWithCapturedArguments(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
def cleanup(name, suffix = ""):
    print("cleanup " + name + suffix)

defer(cleanup, "first")
defer(cleanup, "second", suffix = "!")
defer(lambda: print("third"))
print("body")
`)
	require.NoError(t, err)
	assert.Equal(t, "body\nthird\ncleanup second!\ncleanup first\n", stdout)
}

func TestDeferRunsAfterFail(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
defer(lambda: print("cleanup"))
fail("boom")
print("unreachable")
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "boom")
	assert.Equal(t, "cleanup\n", stdout)
}

func TestDeferredFailureFailsTheScriptButOthersStillRun(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
defer(lambda: print("outermost still runs"))
defer(lambda: fail("cleanup broke"))
print("body")
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "deferred lambda failed: cleanup broke")
	assert.Equal(t, "body\noutermost still runs\n", stdout)
}

func TestMultipleDeferredFailuresPreserveClassificationAndCauses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		secondCleanup string
		secondMessage string
		message       string
	}{
		{
			name:          "intentional failures",
			secondCleanup: `fail("second cleanup rejected")`,
			secondMessage: "second cleanup rejected",
			message:       "2 deferred calls failed",
		},
		{
			name:          "mixed runtime and intentional failures",
			secondCleanup: "1 // 0",
			secondMessage: "division by zero",
			message:       prefixText + ": 2 deferred calls failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stdout, _, err := executeWithStreams(t, `
defer(lambda: print("outermost still runs"))
defer(lambda: fail("first cleanup rejected"))
defer(lambda: `+tc.secondCleanup+`)
print("body")
`)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			require.ErrorIs(t, err, errUtils.ErrScriptRaised)
			assert.EqualError(t, err, tc.message)
			assert.Equal(t, "body\noutermost still runs\n", stdout)
			var aggregate *failure
			require.ErrorAs(t, err, &aggregate)
			causes := aggregate.Unwrap()
			require.Len(t, causes, 2, "both cleanup failures must remain available to callers")
			assert.ErrorContains(t, causes[0], tc.secondMessage)
			assert.ErrorContains(t, causes[1], "first cleanup rejected")
		})
	}
}

func TestDeferredFailureDoesNotMaskTheBodyError(t *testing.T) {
	t.Parallel()
	_, _, err := executeWithStreams(t, `
defer(lambda: fail("cleanup broke"))
fail("boom")
`)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "boom")
	var f *failure
	require.ErrorAs(t, err, &f)
	assert.Contains(t, f.ErrorDetail(), "cleanup broke", "the deferred failure is reported after the body error")
}

func TestDeferRegisteredWhileUnwindingRuns(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
def outer():
    defer(lambda: print("inner"))
    print("outer")

defer(outer)
`)
	require.NoError(t, err)
	assert.Equal(t, "outer\ninner\n", stdout)
}

func TestDeferIsScopedToTheTaskThatRegisteredIt(t *testing.T) {
	t.Parallel()
	stdout, _, err := executeWithStreams(t, `
def work(name):
    defer(lambda: print("cleanup " + name))
    print("work " + name)

defer(lambda: print("main cleanup"))
steps.parallel(tasks = [steps.task(name = n, function = work, args = [n]) for n in ["a", "b"]], max_concurrency = 1)
print("after parallel")
`)
	require.NoError(t, err)
	assert.Equal(t, "[a] work a\n[a] cleanup a\n[b] work b\n[b] cleanup b\nafter parallel\nmain cleanup\n", stdout)
}

func TestDeferRunsAfterCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		cancel()
		<-ctx.Done()
		return process.Result{Err: ctx.Err()}
	})
	var out, errOut bytes.Buffer
	_, err := New(WithProcessRunner(runner)).Execute(ctx, script.Spec{Name: "test.star", Stdout: &out, Stderr: &errOut, Source: `
defer(lambda: print("cleanup after cancel"))
exec.run(["block"])
print("unreachable")
`})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, "cleanup after cancel\n", out.String())
}

func TestDeferValidation(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`defer()`, `defer(1)`, `defer("x")`} {
		_, err := runSource(t, source)
		require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument, source)
	}
}

// interruptHarness replaces the SIGINT source so a test can interrupt a script without a signal.
// Tests that use it must not run in parallel: it swaps package-level hooks.
type interruptHarness struct {
	deliver chan<- os.Signal
	forced  chan struct{}
}

func installInterruptHarness(t *testing.T) *interruptHarness {
	t.Helper()
	h := &interruptHarness{forced: make(chan struct{}, 1)}
	previousNotify, previousForce := notifyInterrupt, forceExit
	notifyInterrupt = func(c chan<- os.Signal) func() {
		h.deliver = c
		return func() {}
	}
	forceExit = func() { h.forced <- struct{}{} }
	t.Cleanup(func() { notifyInterrupt, forceExit = previousNotify, previousForce })
	return h
}

// blockingRunner starts a process that runs until its context is cancelled.
func blockingRunner(t *testing.T, started chan<- struct{}) *MockRunner {
	t.Helper()
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		assert.True(t, signals.InterruptExitSuspended(), "Atmos must not exit on SIGINT while the script runs")
		close(started)
		<-ctx.Done()
		return process.Result{Err: ctx.Err()}
	})
	return runner
}

func TestInterruptCancelsTheScriptAndRunsDeferredCalls(t *testing.T) {
	h := installInterruptHarness(t)
	started := make(chan struct{})
	go func() {
		<-started
		h.deliver <- os.Interrupt
	}()
	var out, errOut bytes.Buffer
	runner := blockingRunner(t, started)
	_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{Name: "test.star", Stdout: &out, Stderr: &errOut, Source: `
defer(lambda: print("cleanup after interrupt"))
exec.run(["block"])
print("unreachable")
`})
	require.ErrorIs(t, err, errUtils.ErrScriptInterrupted)
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 130, exitErr.Code)
	assert.True(t, exitErr.Silent, "an interrupt the user asked for is not an error box")
	assert.Equal(t, "cleanup after interrupt\n", out.String())
	assert.False(t, signals.InterruptExitSuspended(), "the process exit on SIGINT is restored when the session ends")
}

func TestTerminalInterruptCancelsTheScriptAndRunsDeferredCalls(t *testing.T) {
	installInterruptHarness(t)
	var out, errOut bytes.Buffer
	s := newSession(t.Context(), New(), &script.Spec{Name: "test.star", WorkingDirectory: t.TempDir(), Stdout: &out, Stderr: &errOut, Source: `
defer(lambda: print("cleanup after terminal interrupt"))
terminal_interrupt()
print("unreachable")
`})
	s.globals["terminal_interrupt"] = starlark.NewBuiltin("terminal_interrupt", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		s.interrupt.request()
		return nil, s.ctx.Err()
	})
	_, err := s.execute(t.Context())
	require.ErrorIs(t, err, errUtils.ErrScriptInterrupted)
	var exitErr errUtils.ExitCodeError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 130, exitErr.Code)
	assert.True(t, exitErr.Silent)
	assert.Equal(t, "cleanup after terminal interrupt\n", out.String())
}

func TestInterruptReportsAFailingCleanupWithStatus130(t *testing.T) {
	h := installInterruptHarness(t)
	started := make(chan struct{})
	go func() {
		<-started
		h.deliver <- os.Interrupt
	}()
	runner := blockingRunner(t, started)
	_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{Name: "test.star", Source: `
defer(lambda: fail("cleanup failed"))
exec.run(["block"])
`})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	require.ErrorContains(t, err, "cleanup failed")
	assert.Equal(t, 130, errUtils.GetExitCode(err))
}

func TestSecondInterruptExitsAtOnce(t *testing.T) {
	h := installInterruptHarness(t)
	started := make(chan struct{})
	release := make(chan struct{})
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
		close(started)
		<-release
		return process.Result{Err: ctx.Err()}
	})
	go func() {
		<-started
		h.deliver <- os.Interrupt
		h.deliver <- os.Interrupt
		select {
		case <-h.forced:
		case <-time.After(10 * time.Second):
		}
		close(release)
	}()
	_, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{Name: "test.star", Source: `exec.run(["block"])`})
	require.ErrorIs(t, err, errUtils.ErrScriptInterrupted)
	select {
	case <-h.forced:
		t.Fatal("force exit must be reported exactly once")
	default:
	}
}

// The recovery only applies to an interrupt: an ordinary failure keeps its message and status.
func TestFailureWithoutInterruptIsNotAnInterrupt(t *testing.T) {
	installInterruptHarness(t)
	_, err := New().Execute(t.Context(), script.Spec{Name: "test.star", Source: `fail("plain failure")`})
	require.ErrorContains(t, err, "plain failure")
	assert.NotErrorIs(t, err, errUtils.ErrScriptInterrupted)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
}

func TestDeferCancellationDuringCleanupStillRunsRemainingCalls(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runner := NewMockRunner(gomock.NewController(t))
	gomock.InOrder(
		runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
			cancel()
			return process.Result{Err: ctx.Err()}
		}),
		runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ process.TaskSpec) process.Result {
			assert.NoError(t, ctx.Err(), "later cleanup uses a live grace context")
			_, bounded := ctx.Deadline()
			assert.True(t, bounded, "cleanup still has a deadline")
			return process.Result{}
		}),
	)
	var out bytes.Buffer
	_, err := New(WithProcessRunner(runner)).Execute(ctx, script.Spec{Name: "test.star", Stdout: &out, Source: `
def cleanup():
    exec.run(["cleanup"])
    print("cleanup completed")
defer(cleanup)
defer(lambda: exec.run(["cancel"]))
`})
	require.Error(t, err, "the canceled first cleanup is still reported")
	assert.Equal(t, "cleanup completed\n", out.String())
}
