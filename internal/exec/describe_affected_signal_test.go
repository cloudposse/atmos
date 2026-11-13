package exec

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
			original := exitProcess
			exitProcess = func(code int) { exited <- code }
			t.Cleanup(func() { exitProcess = original })
			started := make(chan struct{})
			release := make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			var cleaned atomic.Int32
			stopFirst := removeOnSignal(func() { cleaned.Add(1) })
			stopSecond := removeOnSignal(func() {
				close(started)
				<-release
				cleaned.Add(1)
			})
			// Unblock before stopping even when an assertion fails.
			defer stopFirst()
			defer stopSecond()
			defer unblock()
			affectedSignalCleanups.Lock()
			signals := affectedSignalCleanups.watcher.signals
			affectedSignalCleanups.Unlock()
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
	original := exitProcess
	exitProcess = func(code int) { exited <- code }
	t.Cleanup(func() { exitProcess = original })
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
	stop := removeOnSignal(cleanup)
	defer stop()
	defer unblock()
	go func() {
		cleanup()
		stop()
		close(finished)
	}()
	<-started
	affectedSignalCleanups.Lock()
	signals := affectedSignalCleanups.watcher.signals
	affectedSignalCleanups.Unlock()
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
	// Normal release may win the lock and retire the watcher before it handles
	// the queued signal; otherwise the watcher must preserve the signal status.
	select {
	case code := <-exited:
		require.Equal(t, 128+int(syscall.SIGTERM), code)
	default:
	}
}

func TestRemoveOnSignalReleaseKeepsOtherWorktreesRegistered(t *testing.T) {
	var cleaned atomic.Int32
	stopFirst := removeOnSignal(func() { cleaned.Add(1) })
	stopSecond := removeOnSignal(func() { cleaned.Add(1) })
	stopFirst()
	stopFirst() // Repeated release must not close a watcher twice.
	affectedSignalCleanups.Lock()
	remaining := len(affectedSignalCleanups.watcher.cleanups)
	affectedSignalCleanups.Unlock()
	require.Equal(t, 1, remaining)
	stopSecond()
	affectedSignalCleanups.Lock()
	watcher := affectedSignalCleanups.watcher
	affectedSignalCleanups.Unlock()
	require.Nil(t, watcher)
	require.Zero(t, cleaned.Load())
}
