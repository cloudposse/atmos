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

// interrupt is a context that ends on SIGINT or SIGTERM, like signal.NotifyContext, and also
// remembers which signal arrived so Atmos can exit with the matching shell status.
type interrupt struct {
	ctx      context.Context
	cancel   context.CancelFunc
	signals  chan os.Signal
	received atomic.Value
}

func newInterrupt(parent context.Context) *interrupt {
	ctx, cancel := context.WithCancel(parent)
	i := &interrupt{ctx: ctx, cancel: cancel, signals: make(chan os.Signal, 1)}
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
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func WithCallTimeout(timeout time.Duration) Option {
	return func(o *options) { o.callTimeout = timeout }
}

// callContext merges the caller's context, the handle's interrupt context, and the
// configured call timeout so cancelling any of them cancels the invocation.
func (h *Handle) callContext(ctx context.Context) (context.Context, context.CancelFunc) {
	callCtx, cancel := context.WithCancel(ctx)
	cancelTimeout := func() {}
	if h.callTimeout > 0 {
		var timeoutCancel context.CancelFunc
		callCtx, timeoutCancel = context.WithTimeout(callCtx, h.callTimeout)
		cancelTimeout = timeoutCancel
	}
	stop := func() bool { return false }
	if h.interrupt != nil {
		stop = context.AfterFunc(h.interrupt.ctx, cancel)
	}
	return callCtx, func() {
		stop()
		cancelTimeout()
		cancel()
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
func (h *Handle) finishInvocation(callCtx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if h.interrupt != nil && h.interrupt.fired() {
		h.Close()
		ui.Warning("Interrupted; the plugin process was stopped.")
		return errors.Join(context.Canceled, errUtils.ExitCodeError{Code: h.interrupt.exitCode(), Silent: true})
	}
	if deadlineExceeded(callCtx, err) {
		h.Close()
		return timeoutError(callCtx)
	}
	return err
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

func timeoutError(ctx context.Context) error {
	builder := errUtils.Build(errors.Join(errUtils.ErrPluginTimeout, context.DeadlineExceeded)).
		WithHint("The plugin process was stopped when the deadline passed. Run the command again; if the plugin legitimately needs more time, raise its timeout.")
	if deadline, ok := ctx.Deadline(); ok {
		builder.WithContext("deadline", deadline.Format(time.RFC3339))
	}
	return builder.Err()
}

// startTimeoutError reports a plugin that did not complete its handshake in time.
func startTimeoutError(cause error) error {
	return errUtils.Build(errors.Join(errUtils.ErrPluginTimeout, errUtils.ErrPluginProtocol, cause)).
		WithHint("The plugin process did not finish starting. Check that the executable runs on this platform, then try again.").
		Err()
}

// interruptSignals are the signals that stop a running plugin before Atmos exits.
func interruptSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
