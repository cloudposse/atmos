package git

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRemoveOnSignalWaitsForAllWorktrees(t *testing.T) {
	for _, received := range []os.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(received.String(), func(t *testing.T) {
			exited := make(chan int, 1)
			cleanups := NewWorktreeSignalCleanups(func(code int) { exited <- code })
			started := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			var cleaned atomic.Int32
			stopFirst := cleanups.Register(func() { cleaned.Add(1) })
			stopSecond := cleanups.Register(func() {
				close(started)
				<-release
				cleaned.Add(1)
			})
			// Unblock before stopping even when an assertion fails.
			defer stopFirst()
			defer stopSecond()
			defer unblock()
			cleanups.mu.Lock()
			signals := cleanups.watcher.signals
			cleanups.mu.Unlock()
			signals <- received
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("cleanup did not start")
			}
			select {
			case code := <-exited:
				t.Fatalf("exited with %d while another worktree cleanup was blocked", code)
			case <-time.After(20 * time.Millisecond):
			}
			unblock()
			select {
			case code := <-exited:
				require.Equal(t, 128+int(received.(syscall.Signal)), code)
				require.Equal(t, int32(2), cleaned.Load())
			case <-time.After(10 * time.Second):
				t.Fatal("cleanup did not finish")
			}
		})
	}
}

func TestRemoveOnSignalWaitsForNormalCleanup(t *testing.T) {
	exited := make(chan int, 1)
	cleanups := NewWorktreeSignalCleanups(func(code int) { exited <- code })
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	var cleaned atomic.Int32
	cleanup := sync.OnceFunc(func() {
		close(started)
		<-release
		cleaned.Add(1)
	})
	stop := cleanups.Register(cleanup)
	defer stop()
	defer unblock()
	go func() {
		cleanup()
		stop()
		close(finished)
	}()
	<-started
	cleanups.mu.Lock()
	signals := cleanups.watcher.signals
	cleanups.mu.Unlock()
	signals <- syscall.SIGTERM
	select {
	case code := <-exited:
		t.Fatalf("exited with %d before normal cleanup finished", code)
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("normal cleanup did not finish")
	}
	require.Equal(t, int32(1), cleaned.Load())
	// Normal release may retire the watcher before it handles the queued signal; the received
	// signal must still end the process with its status.
	select {
	case code := <-exited:
		require.Equal(t, 128+int(syscall.SIGTERM), code)
	case <-time.After(10 * time.Second):
		t.Fatal("a received SIGTERM must terminate the process after the final release")
	}
}

// A signal that races the final release still terminates the process, whichever wins the lock,
// and the cleanup never runs twice.
func TestRemoveOnSignalAfterFinalReleaseStillExits(t *testing.T) {
	exited := make(chan int, 1)
	cleanups := NewWorktreeSignalCleanups(func(code int) { exited <- code })
	var cleaned atomic.Int32
	stop := cleanups.Register(func() { cleaned.Add(1) })
	cleanups.mu.Lock()
	watcher := cleanups.watcher
	// Queue the signal while holding the lock, so the watcher and the release below race for it.
	watcher.signals <- syscall.SIGTERM
	cleanups.mu.Unlock()
	stop()
	select {
	case code := <-exited:
		require.Equal(t, 128+int(syscall.SIGTERM), code)
	case <-time.After(10 * time.Second):
		t.Fatal("a signal received before the final release must still terminate the process")
	}
	require.LessOrEqual(t, cleaned.Load(), int32(1), "a cleanup runs at most once")
}

func TestRemoveOnSignalReleaseKeepsOtherWorktreesRegistered(t *testing.T) {
	cleanups := NewWorktreeSignalCleanups(func(code int) { t.Errorf("unexpected exit %d", code) })
	var cleaned atomic.Int32
	stopFirst := cleanups.Register(func() { cleaned.Add(1) })
	stopSecond := cleanups.Register(func() { cleaned.Add(1) })
	stopFirst()
	stopFirst() // Repeated release must not close a watcher twice.
	cleanups.mu.Lock()
	remaining := len(cleanups.watcher.cleanups)
	cleanups.mu.Unlock()
	require.Equal(t, 1, remaining)
	stopSecond()
	cleanups.mu.Lock()
	watcher := cleanups.watcher
	cleanups.mu.Unlock()
	require.Nil(t, watcher)
	require.Zero(t, cleaned.Load())
}

func TestRemoveOnSignalCleansUpBeforeExit(t *testing.T) {
	exited := make(chan int, 1)
	cleanups := NewWorktreeSignalCleanups(func(code int) { exited <- code })

	var cleaned atomic.Int32
	stop := cleanups.Register(func() { cleaned.Add(1) })
	defer stop()
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		t.Skipf("signals cannot be delivered on this platform: %v", err)
	}
	select {
	case code := <-exited:
		require.Equal(t, 128+int(syscall.SIGTERM), code)
		require.Equal(t, int32(1), cleaned.Load(), "the worktree is removed before the process exits")
	case <-time.After(10 * time.Second):
		t.Fatal("signal was not handled")
	}
}

func TestRemoveOnSignalStopReleasesWatcher(t *testing.T) {
	cleanups := NewWorktreeSignalCleanups(func(code int) { t.Errorf("unexpected exit %d", code) })
	var cleaned atomic.Int32
	cleanups.Register(func() { cleaned.Add(1) })()
	// After a normal return the watcher is gone: it must never run cleanup on its own.
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, int32(0), cleaned.Load())
}
