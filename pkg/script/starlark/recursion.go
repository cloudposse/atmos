package starlark

import (
	"errors"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// The maximum number of nested call frames a script may reach before it is stopped.
	// The interpreter itself only stops recursion at 100,000 frames, which produces a six-figure
	// traceback; 10,000 frames is far beyond what any legitimate script needs. The limit is enforced when the
	// interpreter's step budget is exhausted (every recursionCheckSteps steps), so the depth at
	// which a runaway script stops can exceed the limit by up to a few hundred frames.
	maxCallDepth = 10_000

	// The number of interpreter steps between call depth checks. The check
	// is a single comparison, so the interval only trades detection latency for overhead.
	recursionCheckSteps = 1 << 12

	// The user-facing description of the circuit breaker tripping.
	recursionMessage = "recursion depth exceeded (10000 frames)"

	// The prefix the interpreter puts on the message of a cancelled thread.
	recursionCancelPrefix = "Starlark computation cancelled: "

	// The message the interpreter reports when its own hard-coded stack limit trips.
	starlarkOverflowMessage = "Starlark stack overflow"

	recursionHint = "A function keeps calling itself. Check that recursive functions reach a base case, or use a loop."
)

// limitRecursion makes the thread stop with a cancellation when its call stack grows beyond
// maxCallDepth. The interpreter has no per-call depth hook, so the depth is sampled from the step
// budget callback; between samples the budget is simply extended by another chunk. The callback
// runs on the interpreter goroutine, so it can read the stack without synchronization.
func limitRecursion(t *starlark.Thread) {
	t.OnMaxSteps = func(t *starlark.Thread) {
		if t.CallStackDepth() > maxCallDepth {
			t.Cancel(recursionMessage)
			return
		}
		t.SetMaxExecutionSteps(t.Steps + recursionCheckSteps)
	}
	t.SetMaxExecutionSteps(recursionCheckSteps)
}

// isRecursionError reports whether err is a recursion limit failure: either already classified
// as one, or an evaluation error caused by the circuit breaker's cancellation or by starlark-go's
// built-in stack overflow.
func isRecursionError(err error) bool {
	if errors.Is(err, errUtils.ErrStarlarkRecursionLimit) {
		return true
	}
	var eval *starlark.EvalError
	if !errors.As(err, &eval) {
		return false
	}
	return eval.Msg == starlarkOverflowMessage || eval.Msg == recursionCancelPrefix+recursionMessage
}

// recursionFailure classifies err as a recursion limit failure that still wraps the original error.
func recursionFailure(err error) error {
	return failWith(errUtils.ErrStarlarkRecursionLimit, err, "%s", recursionMessage)
}

// evalSummary returns the one-line message of an evaluation error, replacing the interpreter's
// recursion messages with the circuit breaker's description.
func evalSummary(err error) string {
	if isRecursionError(err) {
		return recursionMessage
	}
	return evalMessage(err)
}
