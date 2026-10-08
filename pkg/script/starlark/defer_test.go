package starlark

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
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
	assert.Contains(t, err.Error(), "deferred lambda failed: fail: cleanup broke")
	assert.Equal(t, "body\noutermost still runs\n", stdout)
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
