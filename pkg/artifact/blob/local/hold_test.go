package local

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

// waitTimeout bounds every wait on another goroutine so a broken product path fails the test instead of hanging it.
const waitTimeout = 10 * time.Second

// holdGuard takes the put guard for a test and returns a release that is safe to call any number
// of times. The guard is also released when the test ends, so an assertion that aborts the test can
// never leave the root lock held for the next subtest.
func holdGuard(t *testing.T, s *Store, key string, conditional bool) func() {
	t.Helper()
	release, err := s.guard(context.Background(), blob.OpPut, key, conditional)
	require.NoError(t, err)
	return onceRelease(t, release)
}

// onceRelease wraps a release function that is not idempotent so it runs once, at the first explicit
// call or at test cleanup, whichever comes first.
func onceRelease(t *testing.T, release func()) func() {
	t.Helper()
	var once sync.Once
	wrapped := func() { once.Do(release) }
	t.Cleanup(wrapped)
	return wrapped
}

// closeOnCleanup closes a handle when the test ends, ignoring the error of a second close.
func closeOnCleanup(t *testing.T, root *os.Root) {
	t.Helper()
	t.Cleanup(func() { _ = root.Close() })
}

// receive waits for one value from ch and fails the test, instead of hanging it, when nothing arrives.
func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(waitTimeout):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func TestHoldGuardReleasesWhenTheTestEnds(t *testing.T) {
	s := newStore(t, WithPlainFileCompatibility(), WithLockAllWrites())

	t.Run("a subtest that never releases", func(t *testing.T) {
		explicit := holdGuard(t, s, "held", true)
		explicit()
		explicit() // A second release is a no-op rather than a double unlock.
		_ = holdGuard(t, s, "held", true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release, err := s.guard(ctx, blob.OpPut, "held", true)
	require.NoError(t, err, "the lock a finished subtest still held is free for the next one")
	release()
}
