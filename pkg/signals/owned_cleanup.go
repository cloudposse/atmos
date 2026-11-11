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

var cleanupExitDeferrals atomic.Int32

type ownedCleanupKey struct{}

// DeferExitForCleanup cancels an operation on SIGINT/SIGTERM while letting its
// owner finish bounded credential cleanup before main's immediate exit path.
// The owner must release the scope after cleanup, including error paths.
func DeferExitForCleanup(ctx context.Context) (context.Context, func()) {
	defer perf.Track(nil, "signals.DeferExitForCleanup")()
	operation, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	cleanupExitDeferrals.Add(1)
	var once sync.Once
	return operation, func() {
		once.Do(func() {
			stop()
			cleanupExitDeferrals.Add(-1)
		})
	}
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
