package signals

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

var (
	cleanupExitDeferrals atomic.Int32
	pendingExitCode      atomic.Int32
)

var cleanupScopes = struct {
	sync.Mutex
	next    uint64
	cancel  map[uint64]context.CancelFunc
	drained chan struct{}
}{cancel: make(map[uint64]context.CancelFunc)}

const (
	maxCleanupShutdownWait = 90 * time.Second
	signalExitCodeBase     = 128
)

type ownedCleanupKey struct{}

// DeferExitForCleanup cancels an operation on SIGINT/SIGTERM while letting its
// owner finish bounded credential cleanup before main's immediate exit path.
// The owner must release the scope after cleanup, including error paths.
func DeferExitForCleanup(ctx context.Context) (context.Context, func()) {
	defer perf.Track(nil, "signals.DeferExitForCleanup")()
	operation, cancel := context.WithCancel(ctx)
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	cleanupScopes.Lock()
	if len(cleanupScopes.cancel) == 0 {
		cleanupScopes.drained = make(chan struct{})
	}
	cleanupScopes.next++
	id := cleanupScopes.next
	cleanupScopes.cancel[id] = cancel
	cleanupExitDeferrals.Add(1)
	cleanupScopes.Unlock()
	if ExitRequested() {
		cancel()
	}
	listenerDone := make(chan struct{})
	go func() {
		defer close(listenerDone)
		select {
		case received := <-interrupts:
			RequestExit(received)
		case <-operation.Done():
		}
	}()
	var once sync.Once
	return operation, func() {
		once.Do(func() {
			stopCleanupListener(interrupts, cancel, listenerDone)
			cleanupScopes.Lock()
			delete(cleanupScopes.cancel, id)
			cleanupExitDeferrals.Add(-1)
			if len(cleanupScopes.cancel) == 0 {
				close(cleanupScopes.drained)
			}
			cleanupScopes.Unlock()
		})
	}
}

// stopCleanupListener joins signal delivery before the scope can release its
// exit deferral. A signal already delivered must survive a racing cancellation.
func stopCleanupListener(interrupts chan os.Signal, cancel context.CancelFunc, done <-chan struct{}) {
	signal.Stop(interrupts)
	cancel()
	<-done
	select {
	case received := <-interrupts:
		RequestExit(received)
	default:
	}
}

// RequestExit retains the first signal and cancels every active operation.
// A command that catches a per-secret error cannot turn interruption into success.
func RequestExit(received os.Signal) {
	defer perf.Track(nil, "signals.RequestExit")()

	code := signalExitCodeBase + int(syscall.SIGINT)
	if value, ok := received.(syscall.Signal); ok {
		code = signalExitCodeBase + int(value)
	}
	pendingExitCode.CompareAndSwap(0, int32(code))
	cleanupScopes.Lock()
	cancel := make([]context.CancelFunc, 0, len(cleanupScopes.cancel))
	for _, stop := range cleanupScopes.cancel {
		cancel = append(cancel, stop)
	}
	cleanupScopes.Unlock()
	for _, stop := range cancel {
		stop()
	}
}

// ExitRequested reports a retained shutdown request, including after scopes drain.
//
//nolint:lintroller // Pure atomic state lookup without I/O.
func ExitRequested() bool { return pendingExitCode.Load() != 0 }

// WaitForCleanup waits before plugin/process exit cleanup kills owned issuers.
// The bound also covers an operation that fails to honor cancellation.
func WaitForCleanup() bool {
	defer perf.Track(nil, "signals.WaitForCleanup")()

	if !ExitRequested() {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxCleanupShutdownWait)
	defer cancel()
	return waitForCleanup(ctx)
}

func waitForCleanup(ctx context.Context) bool {
	cleanupScopes.Lock()
	drained := cleanupScopes.drained
	active := len(cleanupScopes.cancel) != 0
	cleanupScopes.Unlock()
	if !active {
		return true
	}
	select {
	case <-drained:
		return true
	case <-ctx.Done():
		return false
	}
}

// ResolveExitCode preserves interruption even if command code swallowed an error
// or raced the main signal handler after finishing its final cleanup.
func ResolveExitCode(fallback int) int {
	defer perf.Track(nil, "signals.ResolveExitCode")()

	if !ExitRequested() {
		return fallback
	}
	WaitForCleanup()
	return int(pendingExitCode.Load())
}

// CleanupExitDeferred reports whether bounded credential cleanup owns shutdown.
//
//nolint:lintroller // Pure atomic state lookup without I/O.
func CleanupExitDeferred() bool { return cleanupExitDeferrals.Load() > 0 }

// OwnedCleanupContext retains custody of a credential acquisition or release
// after caller cancellation. Native plugin supervision honors this bounded
// host-owned context instead of independently killing it on the same signal.
func OwnedCleanupContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	defer perf.Track(nil, "signals.OwnedCleanupContext")()
	return context.WithTimeout(context.WithValue(context.WithoutCancel(ctx), ownedCleanupKey{}, true), timeout)
}

// OwnsCleanup is host-only context metadata, never plugin-controlled settings.
//
//nolint:lintroller // Pure context value lookup without I/O.
func OwnsCleanup(ctx context.Context) bool {
	owned, _ := ctx.Value(ownedCleanupKey{}).(bool)
	return owned
}
