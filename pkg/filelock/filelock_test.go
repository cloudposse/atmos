package filelock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const crossProcessLockTimeout = 5 * time.Second

func TestExclusiveLockHonorsContextAndReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- New(path).WithExclusive(context.Background(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	requireLockSignal(t, held, done, "exclusive lock was not acquired")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := New(path).WithExclusive(ctx, func() error { t.Fatal("lock must not be acquired"); return nil })
	require.ErrorIs(t, err, ErrAcquire)

	close(release)
	requireLockComplete(t, done)
	require.Eventually(t, func() bool {
		return New(path).WithExclusive(context.Background(), func() error { return nil }) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestSharedLocksCanOverlapButBlockExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	firstHeld := make(chan struct{})
	secondHeld := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		firstDone <- New(path).WithShared(context.Background(), func() error {
			close(firstHeld)
			<-release
			return nil
		})
	}()
	requireLockSignal(t, firstHeld, firstDone, "first shared lock was not acquired")

	go func() {
		secondDone <- New(path).WithShared(context.Background(), func() error {
			close(secondHeld)
			<-release
			return nil
		})
	}()
	requireLockSignal(t, secondHeld, secondDone, "shared locks should not block one another")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := New(path).WithExclusive(ctx, func() error {
		return errors.New("exclusive lock must not be acquired")
	})
	require.ErrorIs(t, err, ErrAcquire)

	close(release)
	requireLockComplete(t, firstDone)
	requireLockComplete(t, secondDone)
	require.Eventually(t, func() bool {
		return New(path).WithExclusive(context.Background(), func() error { return nil }) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestLockFilePersistsAndUnlocksAfterCallbackError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	want := errors.New("update failed")
	err := New(path).WithExclusive(context.Background(), func() error { return want })
	require.ErrorIs(t, err, want)

	_, err = os.Stat(path)
	require.NoError(t, err, "the stable sibling lock file must remain after release")
	require.NoError(t, New(path).WithExclusive(context.Background(), func() error { return nil }))
}

// processWaitTimeout bounds how long the test waits for the helper subprocess to start
// or exit. Windows CI runners have much heavier process creation/teardown overhead than
// Unix, especially under load from the rest of the suite's other packages running
// concurrently; a 1s margin was observed to flake there ("helper process did not release
// the lock") even though the helper's own critical section only holds the lock 300ms.
const processWaitTimeout = 5 * time.Second

func TestExclusiveLockContendsAcrossProcesses(t *testing.T) {
	if os.Getenv("ATMOS_FILELOCK_HELPER") == "1" {
		path := os.Getenv("ATMOS_FILELOCK_PATH")
		readyPath := os.Getenv("ATMOS_FILELOCK_READY_PATH")
		releasePath := os.Getenv("ATMOS_FILELOCK_RELEASE_PATH")
		if err := New(path).WithExclusive(context.Background(), func() error {
			if writeErr := os.WriteFile(readyPath, []byte("locked"), 0o600); writeErr != nil {
				return fmt.Errorf("write ready signal: %w", writeErr)
			}
			return waitForFile(releasePath, crossProcessLockTimeout)
		}); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "cross-process.lock")
	readyPath := filepath.Join(tempDir, "helper-ready")
	releasePath := filepath.Join(tempDir, "helper-release")
	cmd := exec.Command(os.Args[0], "-test.run=TestExclusiveLockContendsAcrossProcesses")
	cmd.Env = append(
		os.Environ(),
		"ATMOS_FILELOCK_HELPER=1",
		"ATMOS_FILELOCK_PATH="+path,
		"ATMOS_FILELOCK_READY_PATH="+readyPath,
		"ATMOS_FILELOCK_RELEASE_PATH="+releasePath,
	)
	var stderr synchronizedBuffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	require.Eventually(t, func() bool {
		_, statErr := os.Stat(readyPath)
		return statErr == nil
	}, crossProcessLockTimeout, 10*time.Millisecond, "helper process did not acquire the lock: %s", stderr.String())
	ready, err := os.ReadFile(readyPath)
	require.NoError(t, err)
	require.Equal(t, []byte("locked"), ready)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err = New(path).WithExclusive(ctx, func() error { return errors.New("must not run") })
	require.ErrorIs(t, err, ErrAcquire)
	require.NoError(t, os.WriteFile(releasePath, []byte("release"), 0o600))

	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		waited = true
		require.NoError(t, err, "helper process failed: %s", stderr.String())
	case <-time.After(crossProcessLockTimeout):
		require.NoError(t, cmd.Process.Kill())
		<-wait
		waited = true
		t.Fatalf("helper process did not release the lock: %s", stderr.String())
	}

	require.Eventually(t, func() bool {
		return New(path).WithExclusive(context.Background(), func() error { return nil }) == nil
	}, crossProcessLockTimeout, 10*time.Millisecond, "lock was not released after helper exit")
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect release signal: %w", err)
		}

		select {
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for release signal %q", path)
		case <-ticker.C:
		}
	}
}

type synchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buffer.String()
}

func requireLockSignal(t *testing.T, signal <-chan struct{}, done <-chan error, message string) {
	t.Helper()
	select {
	case <-signal:
	case err := <-done:
		require.NoError(t, err)
		t.Fatal(message)
	case <-time.After(processWaitTimeout):
		t.Fatal(message)
	}
}

func requireLockComplete(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(processWaitTimeout):
		t.Fatal("lock operation did not complete")
	}
}
