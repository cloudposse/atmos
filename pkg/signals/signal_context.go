package signals

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// InterruptContext returns a context that is canceled when SIGINT or SIGTERM
// arrives, like signal.NotifyContext. Its cancellation cause wraps
// errUtils.ErrSignalReceived, so a subprocess runner can tell an interrupt
// (which a graceful child handles itself) from a timeout or another
// cancellation (which still terminates the child). Call stop to release the
// signal listener.
func InterruptContext(parent context.Context) (ctx context.Context, stop context.CancelFunc) {
	defer perf.Track(nil, "signals.InterruptContext")()

	received := make(chan os.Signal, 1)
	signal.Notify(received, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := cancelOnSignal(parent, received)
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			signal.Stop(received)
			cancel()
		})
	}
}

// cancelOnSignal cancels the returned context with a signal cause when received delivers.
func cancelOnSignal(parent context.Context, received <-chan os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	go func() {
		select {
		case sig := <-received:
			cancel(fmt.Errorf("%w: %s", errUtils.ErrSignalReceived, sig))
		case <-ctx.Done():
		}
	}()
	return ctx, func() { cancel(context.Canceled) }
}

// InterruptedBySignal reports whether ctx was canceled because Atmos received
// SIGINT or SIGTERM through InterruptContext, including through derived contexts.
func InterruptedBySignal(ctx context.Context) bool {
	defer perf.Track(nil, "signals.InterruptedBySignal")()

	return ctx.Err() != nil && errors.Is(context.Cause(ctx), errUtils.ErrSignalReceived)
}
