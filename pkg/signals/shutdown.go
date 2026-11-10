package signals

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"

	"github.com/cloudposse/atmos/pkg/perf"
)

const signalExitCodeBase = 128

var (
	shutdownMu      sync.Mutex
	shutdownSeq     int
	shutdownOwners  = map[int]context.CancelCauseFunc{}
	committedSignal os.Signal
)

type shutdownCause struct{ signal os.Signal }

func (c shutdownCause) Error() string {
	defer perf.Track(nil, "signals.shutdownCause.Error")()

	return "interrupted by " + c.signal.String()
}

// WithShutdownContext gives an operation ownership of graceful signal shutdown.
// Release it only after its children have stopped and its resources are cleaned up.
// A signal already committed to hard exit cancels new operations before they start.
func WithShutdownContext(parent context.Context) (context.Context, func()) {
	defer perf.Track(nil, "signals.WithShutdownContext")()

	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancelCause(parent)
	shutdownMu.Lock()
	shutdownSeq++
	id := shutdownSeq
	if committedSignal != nil {
		cancel(shutdownCause{signal: committedSignal})
	} else {
		shutdownOwners[id] = cancel
	}
	shutdownMu.Unlock()

	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			shutdownMu.Lock()
			delete(shutdownOwners, id)
			shutdownMu.Unlock()
			cancel(nil)
		})
	}
}

// HandleShutdownSignal cancels active owners and lets their normal return paths
// perform cleanup and preserve exit codes. False commits the caller to hard exit.
func HandleShutdownSignal(sig os.Signal) bool {
	defer perf.Track(nil, "signals.HandleShutdownSignal")()

	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	if len(shutdownOwners) == 0 {
		committedSignal = sig
		return false
	}
	for _, cancel := range shutdownOwners {
		cancel(shutdownCause{signal: sig})
	}
	return true
}

// CancellationSignal returns the shutdown signal that canceled ctx, if any.
func CancellationSignal(ctx context.Context) os.Signal {
	defer perf.Track(nil, "signals.CancellationSignal")()

	if ctx == nil {
		return nil
	}
	var cause shutdownCause
	if errors.As(context.Cause(ctx), &cause) {
		return cause.signal
	}
	return nil
}

// ExitCodeForSignal returns the conventional shell exit status for a signal.
func ExitCodeForSignal(sig os.Signal) int {
	defer perf.Track(nil, "signals.ExitCodeForSignal")()

	if value, ok := sig.(syscall.Signal); ok {
		return signalExitCodeBase + int(value)
	}
	return signalExitCodeBase + int(syscall.SIGINT)
}
