package starlark

import (
	"context"
	"strings"
	"testing"
	"time"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

// Compile-time sentinel: the tests below depend on this sentinel and the engine constants.
var _ = errUtils.ErrStarlarkRecursionLimit

const unboundedRecursion = `
def f(n):
    return f(n + 1)
f(0)
`

// requireShortRecursionExplanation asserts the rendered explanation collapsed the repeated frames.
func requireShortRecursionExplanation(t *testing.T, err error) {
	t.Helper()
	details := cockroach.GetAllDetails(err)
	require.NotEmpty(t, details)
	joined := strings.Join(details, "\n")
	lines := strings.Count(joined, "\n") + 1
	assert.Less(t, lines, 60, "explanation must be short, got %d lines", lines)
	assert.Contains(t, joined, "more identical frames)")
	assert.Contains(t, joined, "in f")
}

func requireRecursionHint(t *testing.T, err error) {
	t.Helper()
	hints := cockroach.GetAllHints(err)
	require.Len(t, hints, 1)
	assert.Equal(t, recursionHint, hints[0])
}

func TestUnboundedRecursionTripsCircuitBreaker(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, unboundedRecursion)
	require.ErrorIs(t, err, errUtils.ErrStarlarkRecursionLimit)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Equal(t, prefixText+": recursion depth exceeded (10000 frames)", err.Error())
	requireShortRecursionExplanation(t, err)
	requireRecursionHint(t, err)
}

func TestMutualRecursionIsCollapsed(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
def a(n):
    return b(n + 1)
def b(n):
    return a(n + 1)
a(0)
`)
	require.ErrorIs(t, err, errUtils.ErrStarlarkRecursionLimit)
	details := strings.Join(cockroach.GetAllDetails(err), "\n")
	assert.Less(t, strings.Count(details, "\n"), 60)
	assert.Contains(t, details, "more identical frames)")
	assert.Contains(t, details, "in a")
	assert.Contains(t, details, "in b")
}

func TestStarlarkOwnStackOverflowIsClassified(t *testing.T) {
	t.Parallel()
	// A thread built without the circuit breaker falls through to starlark-go's own 100,000 frame limit.
	_, err := starlark.ExecFileOptions(fileOptions, &starlark.Thread{Name: "raw"}, "test.star", unboundedRecursion, nil)
	require.Error(t, err)
	wrapped := scriptError(t.Context(), err, "")
	require.ErrorIs(t, wrapped, errUtils.ErrStarlarkRecursionLimit)
	require.ErrorIs(t, wrapped, errUtils.ErrStarlark)
	assert.Equal(t, prefixText+": recursion depth exceeded (10000 frames)", wrapped.Error())
	requireShortRecursionExplanation(t, wrapped)
	requireRecursionHint(t, wrapped)
}

func TestBoundedDeepRecursionSucceeds(t *testing.T) {
	t.Parallel()
	result, err := runSource(t, `
def depth(n):
    if n == 0:
        return 0
    return 1 + depth(n - 1)
output = depth(5000)
`)
	require.NoError(t, err)
	assert.Equal(t, "5000", result.Value)
}

func TestRecursionInParallelTaskTripsCircuitBreaker(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, `
def f(n):
    return f(n + 1)
def run():
    f(0)
steps.parallel(functions = [run])
`)
	require.ErrorIs(t, err, errUtils.ErrStarlarkRecursionLimit)
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.Contains(t, err.Error(), "recursion depth exceeded (10000 frames)")
	assert.NotContains(t, err.Error(), "cancelled")
	details := strings.Join(cockroach.GetAllDetails(err), "\n")
	assert.Less(t, strings.Count(details, "\n"), 120)
	assert.Contains(t, details, "more identical frames)")
	requireRecursionHint(t, err)
}

func TestOrdinaryRuntimeErrorIsNotClassifiedAsRecursion(t *testing.T) {
	t.Parallel()
	_, err := runSource(t, "def inner(d):\n    return d[\"missing\"]\ndef outer():\n    return inner({})\nouter()")
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	assert.NotErrorIs(t, err, errUtils.ErrStarlarkRecursionLimit)
	assert.Empty(t, cockroach.GetAllHints(err))
	details := cockroach.GetAllDetails(err)
	require.Len(t, details, 1)
	assert.NotContains(t, details[0], "identical frames")
	assert.Equal(t, "```text\n"+
		"Traceback (most recent call last):\n"+
		"  test.star:5:6: in <toplevel>\n"+
		"  test.star:4:17: in outer\n"+
		"  test.star:2:13: in inner\n"+
		"Error: key \"missing\" not in dict\n```", details[0])
}

func TestContextCancellationStillStopsLongLoop(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := New().Execute(ctx, script.Spec{Name: "loop.star", Source: "def spin():\n    for _ in range(1 << 40):\n        pass\nspin()"})
		done <- err
	}()
	select {
	case err := <-done:
		require.ErrorIs(t, err, errUtils.ErrStarlark)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.NotErrorIs(t, err, errUtils.ErrStarlarkRecursionLimit)
	case <-time.After(10 * time.Second):
		t.Fatal("cancelled script did not stop")
	}
}

func TestLongLoopWithShallowStackIsNotLimited(t *testing.T) {
	t.Parallel()
	// Far more steps than one budget chunk: the budget must be extended, never exhausted.
	result, err := runSource(t, "def total():\n    t = 0\n    for i in range(200000):\n        t += i\n    return t\noutput = total()")
	require.NoError(t, err)
	assert.Equal(t, "19999900000", result.Value)
}
