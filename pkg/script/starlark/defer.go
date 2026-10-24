package starlark

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signals"
)

const deferKey = "atmos.starlark.defer"

// deferGracePeriod bounds the deferred calls that run after the script's own context is
// already cancelled, so Ctrl-C still tears down what the script started without hanging.
const deferGracePeriod = 30 * time.Second

// deferred is one registered call: the function plus the arguments captured at registration.
type deferred struct {
	fn     starlark.Callable
	args   starlark.Tuple
	kwargs []starlark.Tuple
}

// deferStack is a thread's list of deferred calls. It runs last-in-first-out when the thread's
// script body or task function finishes, whether that ended normally, with fail(), or by
// cancellation. Threads are single-goroutine, so the stack needs no lock.
type deferStack struct{ calls []*deferred }

func deferStackOf(t *starlark.Thread) *deferStack {
	if stack, ok := t.Local(deferKey).(*deferStack); ok {
		return stack
	}
	stack := &deferStack{}
	t.SetLocal(deferKey, stack)
	return stack
}

// deferCall is the `defer(fn, *args, **kwargs)` builtin. Arguments are captured now and the
// call happens when the enclosing script or task finishes, like Go's defer.
func deferCall(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 {
		return nil, withHint(invalidArg("defer: missing function argument"), `Pass the function to run later, e.g. defer(lambda: atmos.emulator("down", "aws")).`)
	}
	fn, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, invalidArg("defer: got %s, want a function", args[0].Type())
	}
	stack := deferStackOf(t)
	stack.calls = append(stack.calls, &deferred{
		fn:     fn,
		args:   append(starlark.Tuple(nil), args[1:]...),
		kwargs: append([]starlark.Tuple(nil), kwargs...),
	})
	return starlark.None, nil
}

// runDeferred runs the thread's deferred calls last-in-first-out and returns the body error
// combined with any deferred failures. The body error stays the primary error and deferred
// failures are listed after it, so cleanup can never mask what went wrong first. Calls
// registered while unwinding run too.
func (s *session) runDeferred(ctx context.Context, t *starlark.Thread, bodyErr error) error {
	stack, ok := t.Local(deferKey).(*deferStack)
	if !ok || len(stack.calls) == 0 {
		return bodyErr
	}
	thread := t
	stop := func() {}
	defer func() { stop() }()
	var failures []error
	for len(stack.calls) > 0 {
		// Cancellation can arrive during cleanup; switch once and share one grace deadline.
		if thread == t && ctx.Err() != nil {
			thread, stop = s.deferThread(ctx, t)
		}
		last := len(stack.calls) - 1
		call := stack.calls[last]
		stack.calls = stack.calls[:last]
		if _, err := starlark.Call(thread, call.fn, call.args, call.kwargs); err != nil {
			failures = append(failures, failWith(errUtils.ErrStarlark, err, "deferred %s failed: %s", call.fn.Name(), evalSummary(err)))
		}
	}
	return combineDeferred(bodyErr, failures)
}

// combineDeferred merges deferred failures into the script's result.
func combineDeferred(bodyErr error, failures []error) error {
	switch {
	case len(failures) == 0:
		return bodyErr
	case bodyErr == nil && len(failures) == 1:
		return failures[0]
	case bodyErr == nil:
		return failWithAll(errUtils.ErrStarlark, failures, "%d deferred calls failed", len(failures))
	}
	messages := make([]string, 0, len(failures))
	for _, failure := range failures {
		messages = append(messages, failure.Error())
	}
	combined := failWithAll(errUtils.ErrStarlark, append([]error{bodyErr}, failures...), "%s", evalSummary(bodyErr))
	return withDetail(combined, "Deferred calls also failed:\n"+strings.Join(messages, "\n"))
}

// deferThread picks the thread for deferred calls. While the script's context is live they run
// on the thread that registered them. Once it is cancelled that thread refuses further calls,
// so a fresh thread with a bounded grace context runs the cleanup, sharing the original
// thread's output sink, step library, and deferred stack.
func (s *session) deferThread(ctx context.Context, t *starlark.Thread) (*starlark.Thread, func()) {
	if ctx.Err() == nil {
		return t, func() {}
	}
	grace, cancel := context.WithTimeout(context.WithoutCancel(ctx), deferGracePeriod)
	out, _ := t.Local(outputKey).(*taskOutput)
	thread, stop := s.thread(grace, t.Name, out)
	thread.SetLocal(stepLibraryKey, t.Local(stepLibraryKey))
	thread.SetLocal(deferKey, t.Local(deferKey))
	return thread, func() {
		stop()
		cancel()
	}
}

// interruptExitCode is the conventional status of a process ended by SIGINT (128 + 2).
const interruptExitCode = 130

// notifyInterrupt delivers SIGINT to c until the returned function is called. Tests replace it.
var notifyInterrupt = func(c chan<- os.Signal) (stop func()) {
	signal.Notify(c, os.Interrupt)
	return func() { signal.Stop(c) }
}

// forceExit ends the process at once on a second interrupt, for a user who will not wait for the
// deferred calls. Tests replace it.
var forceExit = func() {
	signals.RunExitCleanups()
	errUtils.OsExit(interruptExitCode)
}

// interruptWatch turns Ctrl-C into cancellation of one script session. Atmos normally exits on
// SIGINT before any context is cancelled, which would skip the script's deferred calls. While the
// watch is active that exit is suspended; the first interrupt cancels the session so deferred
// calls run within their grace period, and a second interrupt exits immediately.
type interruptWatch struct {
	cancel   context.CancelFunc
	received chan os.Signal
	stopSig  func()
	release  func()
	count    atomic.Int32
	done     chan struct{}
	stopOnce sync.Once
}

// watchInterrupt starts watching for SIGINT and returns the context the session must run under.
func watchInterrupt(parent context.Context) (context.Context, *interruptWatch) {
	ctx, cancel := context.WithCancel(parent)
	w := &interruptWatch{cancel: cancel, received: make(chan os.Signal, 2), done: make(chan struct{})}
	w.release = signals.SuspendInterruptExit()
	w.stopSig = notifyInterrupt(w.received)
	go w.listen()
	return ctx, w
}

func (w *interruptWatch) listen() {
	for {
		select {
		case <-w.received:
			if w.count.Add(1) > 1 {
				forceExit()
				return
			}
			w.cancel()
		case <-w.done:
			return
		}
	}
}

// interrupted reports whether an interrupt arrived while the session ran.
func (w *interruptWatch) interrupted() bool { return w.count.Load() > 0 }

// stop releases the signal handling and the context. It is safe to call more than once.
func (w *interruptWatch) stop() {
	w.stopOnce.Do(func() {
		w.stopSig()
		close(w.done)
		w.release()
		w.cancel()
	})
}

// finishInterrupted completes a session that an interrupt cancelled. Deferred calls run first,
// within their grace period. A failing cleanup is reported; otherwise the exit is silent, like a
// shell's, because the user asked for it. Either way the status is 130.
func (s *session) finishInterrupted(ctx context.Context, thread *starlark.Thread) error {
	if err := s.runDeferred(ctx, thread, nil); err != nil {
		return errUtils.WithExitCode(scriptError(ctx, err, s.spec.ProjectRoot), interruptExitCode)
	}
	return fmt.Errorf("%w: %w", errUtils.ErrScriptInterrupted, errUtils.ExitCodeError{Code: interruptExitCode, Silent: true})
}
