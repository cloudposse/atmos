package signals

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupOwnsBoundedCancellationAndReleasesExitDeferral(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	operation, release := DeferExitForCleanup(parent)
	t.Cleanup(release)
	require.True(t, CleanupExitDeferred())
	owned, stop := OwnedCleanupContext(operation, time.Second)
	defer stop()
	cancel()
	require.ErrorIs(t, operation.Err(), context.Canceled)
	require.NoError(t, owned.Err(), "cleanup must survive caller cancellation")
	require.True(t, OwnsCleanup(owned))
	require.False(t, OwnsCleanup(parent))
	_, bounded := owned.Deadline()
	require.True(t, bounded)
	release()
	release()
	require.False(t, CleanupExitDeferred())
}

func TestSignalRetainedUntilEveryCredentialOwnerReleases(t *testing.T) {
	t.Cleanup(func() { pendingExitCode.Store(0) })
	first, releaseFirst := DeferExitForCleanup(t.Context())
	t.Cleanup(releaseFirst)
	second, releaseSecond := DeferExitForCleanup(t.Context())
	t.Cleanup(releaseSecond)
	RequestExit(syscall.SIGTERM)
	require.ErrorIs(t, first.Err(), context.Canceled)
	require.ErrorIs(t, second.Err(), context.Canceled)
	require.True(t, ExitRequested())
	RequestExit(os.Interrupt)

	// A command may catch a per-secret cancellation and return success. Exit
	// resolution must still wait for every owner and retain the first signal.
	exit := make(chan int, 1)
	go func() { exit <- ResolveExitCode(0) }()
	assertWaiting := func() {
		t.Helper()
		select {
		case code := <-exit:
			t.Fatalf("exited before every owner released: %d", code)
		case <-time.After(20 * time.Millisecond):
		}
	}
	assertWaiting()
	releaseFirst()
	assertWaiting()
	releaseSecond()
	select {
	case code := <-exit:
		require.Equal(t, 143, code)
	case <-time.After(time.Second):
		t.Fatal("exit did not resume after cleanup")
	}
	require.False(t, CleanupExitDeferred())
	late, releaseLate := DeferExitForCleanup(t.Context())
	t.Cleanup(releaseLate)
	require.ErrorIs(t, late.Err(), context.Canceled, "new work must not start after a retained signal")
}

func TestSignalCleanupWaitIsBounded(t *testing.T) {
	t.Cleanup(func() { pendingExitCode.Store(0) })
	_, release := DeferExitForCleanup(t.Context())
	t.Cleanup(release)
	RequestExit(os.Interrupt)
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	require.False(t, waitForCleanup(ctx))
	release()
	require.True(t, waitForCleanup(t.Context()))
	require.Equal(t, 130, ResolveExitCode(0))
}

func TestUninterruptedExitKeepsCommandStatus(t *testing.T) {
	require.False(t, ExitRequested())
	require.Equal(t, 7, ResolveExitCode(7))
}

func TestReleaseRetainsSignalBufferedBehindCancellation(t *testing.T) {
	t.Cleanup(func() { pendingExitCode.Store(0) })
	interrupts := make(chan os.Signal, 1)
	interrupts <- syscall.SIGTERM
	done := make(chan struct{})
	close(done) // The listener selected operation.Done instead of the signal.
	canceled := false
	stopCleanupListener(interrupts, func() { canceled = true }, done)
	require.True(t, canceled)
	require.Equal(t, 143, ResolveExitCode(0))
}

func TestReleaseJoinsListenerBeforeReturning(t *testing.T) {
	t.Cleanup(func() { pendingExitCode.Store(0) })
	interrupts := make(chan os.Signal, 1)
	done, canceled, released := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		stopCleanupListener(interrupts, func() { close(canceled) }, done)
		close(released)
	}()
	<-canceled
	select {
	case <-released:
		t.Fatal("scope released before its signal listener finished")
	default:
	}
	RequestExit(syscall.SIGTERM) // Simulate a listener that received the signal.
	close(done)
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("release did not resume after the listener finished")
	}
	require.Equal(t, 143, ResolveExitCode(0))
}
