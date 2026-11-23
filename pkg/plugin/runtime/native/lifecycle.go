package native

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	// Termination grace is how long a plugin may take to exit after SIGTERM before it is killed.
	terminationGrace = 3 * time.Second
	// The interrupted exit code is the conventional shell status for a command ended by SIGINT.
	interruptedExitCode = 130
	signalExitBase      = 128
)

// Signal registration is replaced by tests to inject interrupts without signaling the test process.
var (
	notifySignals = signal.Notify
	resetSignals  = signal.Stop
)

// The omitted-logs notice is qualified because the host cannot know how many log events the
// plugin still had queued once the call was cancelled; the plugin's own count travels in the
// response that cancellation discards.
const omittedLogsNotice = "Log events the plugin had not yet delivered may not have been shown."

// Warnings are routed through a variable so tests can observe what reaches the user.
var warn = ui.Warning

// interrupt is a context that ends on SIGINT or SIGTERM, like signal.NotifyContext, and also
// remembers which signal arrived so Atmos can exit with the matching shell status.
type interrupt struct {
	ctx      context.Context
	cancel   context.CancelFunc
	signals  chan os.Signal
	received atomic.Value
}

// newInterrupt cancels on SIGINT or SIGTERM unless the parent already owns
// credential cleanup and must retain control of signal-driven shutdown.
func newInterrupt(parent context.Context) *interrupt {
	ctx, cancel := context.WithCancel(parent)
	i := &interrupt{ctx: ctx, cancel: cancel, signals: make(chan os.Signal, 1)}
	if signals.OwnsCleanup(parent) {
		return i
	}
	notifySignals(i.signals, interruptSignals()...)
	go func() {
		select {
		case received := <-i.signals:
			i.received.Store(received)
			// Stop catching signals so a second one takes the process-level action.
			resetSignals(i.signals)
			cancel()
		case <-ctx.Done():
		}
	}()
	return i
}

// stop releases the signal subscription and the context.
func (i *interrupt) stop() {
	resetSignals(i.signals)
	i.cancel()
}

// exitCode is 128 plus the signal number, the POSIX convention main also uses.
func (i *interrupt) exitCode() int {
	if received, ok := i.received.Load().(syscall.Signal); ok {
		return signalExitBase + int(received)
	}
	return interruptedExitCode
}

func (i *interrupt) fired() bool { return i.received.Load() != nil }

// WithCallTimeout bounds every invocation made through the handle. The deadline is sent to
// the plugin, and the plugin process is stopped when it passes. Zero or negative disables it.
// For an interactive invocation, time spent waiting for the user (see Handle.AwaitUser) does not
// count against the timeout; WithInteractionLimit bounds that wait instead.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithCallTimeout(timeout time.Duration) Option {
	return func(o *options) { o.callTimeout = timeout }
}

// callContext merges the caller's context, the handle's interrupt context, and the
// configured call timeout so cancelling any of them cancels the invocation.
//
// A noninteractive invocation can never wait for a person, so its deadline is the call timeout.
// An interactive one gets a hard deadline of the call timeout plus the interaction limit, which is
// what the plugin sees, and a budget that cancels the call once the plugin has worked for the call
// timeout outside of user waits. The returned budget is nil when no such budget applies.
func (h *Handle) callContext(ctx context.Context, interactive bool) (context.Context, *budget, context.CancelFunc) {
	callCtx, cancel := context.WithCancelCause(ctx)
	limit := h.callTimeout
	var work *budget
	if limit > 0 && interactive {
		work = newBudget(h.callTimeout, h.interactionLimit, func(bool) { cancel(errUtils.ErrPluginTimeout) })
		limit = saturatingDurationAdd(limit, h.interactionLimit)
	}
	cancelTimeout := func() {}
	if limit > 0 {
		var timeoutCancel context.CancelFunc
		deadline := time.Now().Add(limit)
		callCtx, timeoutCancel = context.WithDeadline(callCtx, deadline)
		cancelTimeout = timeoutCancel
		if work != nil {
			work.deadline = deadline
		}
	}
	stop := func() bool { return false }
	if h.interrupt != nil {
		stop = context.AfterFunc(h.interrupt.ctx, func() { cancel(context.Canceled) })
	}
	return callCtx, work, func() {
		stop()
		if work != nil {
			work.stop()
		}
		cancelTimeout()
		cancel(context.Canceled)
	}
}

// invocationDeadline reports the call's deadline to the plugin when one exists.
func invocationDeadline(ctx context.Context) *timestamppb.Timestamp {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}
	return timestamppb.New(deadline)
}

// finishInvocation converts interruption and deadline expiry into actionable errors and
// stops the plugin so it cannot outlive the operation that was abandoned.
// An interactive call that reaches its own hard deadline, rather than an earlier one its caller
// set, spent the interaction limit waiting for the user. That case is reported differently from a
// plugin that ran out of time. The work budget is nil for calls without an interactive budget.
func (h *Handle) finishInvocation(callCtx context.Context, work *budget, err error) error {
	if err == nil {
		return nil
	}
	if h.interrupt != nil && h.interrupt.fired() {
		h.Close()
		warn("Interrupted; the plugin process was stopped.")
		warn(omittedLogsNotice)
		return errors.Join(context.Canceled, errUtils.ExitCodeError{Code: h.interrupt.exitCode(), Silent: true})
	}
	// The interactive budget cancels with ErrPluginTimeout as the cause once the plugin's own
	// work, excluding user waits, has used up the call timeout, or once the user waits have used
	// up the interaction limit.
	if errors.Is(context.Cause(callCtx), errUtils.ErrPluginTimeout) {
		h.Close()
		warn(omittedLogsNotice)
		return h.budgetTimeoutError(work)
	}
	if deadlineExceeded(callCtx, err) {
		h.Close()
		warn(omittedLogsNotice)
		deadline, ok := callCtx.Deadline()
		if work != nil && deadline.Equal(work.deadline) {
			return interactionTimeoutError(deadline, h.interactionLimit, h.packageAlias)
		}
		return timeoutError(deadline, ok)
	}
	// A caller that cancels the call (for example, after a sibling operation failed) gets the
	// error unchanged, but the user still learns that the plugin's queued logs may be missing.
	if errors.Is(callCtx.Err(), context.Canceled) {
		warn(omittedLogsNotice)
	}
	return err
}

// budgetTimeoutError reports why the interactive budget ended the call: the user wait ran out
// (the sign-in error) or the plugin's own work did (the generic timeout error).
func (h *Handle) budgetTimeoutError(work *budget) error {
	if work != nil && work.userWaitExhausted() {
		return interactionTimeoutError(time.Now(), h.interactionLimit, h.packageAlias)
	}
	return timeoutError(time.Now(), true)
}

// deadlineExceeded reports whether a failed call ran out of time. The plugin's gRPC server can
// cancel the stream at the propagated deadline slightly before the caller's own context reports
// expiry, so a DeadlineExceeded status counts whenever the call carried a deadline.
func deadlineExceeded(callCtx context.Context, err error) bool {
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		return true
	}
	_, hasDeadline := callCtx.Deadline()
	return hasDeadline && status.Code(err) == codes.DeadlineExceeded
}

func timeoutError(deadline time.Time, known bool) error {
	builder := errUtils.Build(errors.Join(errUtils.ErrPluginTimeout, context.DeadlineExceeded)).
		WithHint("The plugin process was stopped when the deadline passed. Run the command again; if the plugin legitimately needs more time, raise its timeout.")
	if known {
		builder.WithContext("deadline", deadline.Format(time.RFC3339))
	}
	return builder.Err()
}

// interactionTimeoutError reports an interactive call whose user wait outlasted the interaction limit.
// The hint names the key that raises the limit; alias is empty when the caller did not name the package.
func interactionTimeoutError(deadline time.Time, limit time.Duration, alias string) error {
	if alias == "" {
		alias = "<alias>"
	}
	return errUtils.Build(errors.Join(errUtils.ErrPluginTimeout, context.DeadlineExceeded)).
		WithHintf("Atmos waited %s for the sign-in to complete and then stopped the plugin. Run the command again and finish signing in before the limit, or raise `plugins.packages.%s.interaction_timeout` if the sign-in needs longer.", limit, alias).
		WithContext("deadline", deadline.Format(time.RFC3339)).
		Err()
}

// startTimeoutError reports a plugin that did not complete its handshake in time.
func startTimeoutError(cause error) error {
	return errUtils.Build(errors.Join(errUtils.ErrPluginTimeout, errUtils.ErrPluginProtocol, cause)).
		WithHint("The plugin process did not finish starting. Check that the executable runs on this platform, then try again.").
		Err()
}

// interruptSignals are the signals that stop a running plugin before Atmos exits.
func interruptSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
