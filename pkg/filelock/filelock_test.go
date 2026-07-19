package filelock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
		if err := New(path).WithExclusive(context.Background(), func() error {
			_, _ = os.Stdout.WriteString("locked\n")
			time.Sleep(300 * time.Millisecond)
			return nil
		}); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	path := filepath.Join(t.TempDir(), "cross-process.lock")
	cmd := exec.Command(os.Args[0], "-test.run=TestExclusiveLockContendsAcrossProcesses")
	cmd.Env = append(os.Environ(), "ATMOS_FILELOCK_HELPER=1", "ATMOS_FILELOCK_PATH="+path)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	scanner := bufio.NewScanner(stdout)
	type scanResult struct {
		ok   bool
		text string
		err  error
	}
	ready := make(chan scanResult, 1)
	go func() {
		ready <- scanResult{ok: scanner.Scan(), text: scanner.Text(), err: scanner.Err()}
	}()
	select {
	case result := <-ready:
		require.NoError(t, result.err)
		require.True(t, result.ok)
		require.Equal(t, "locked", result.text)
	case <-time.After(processWaitTimeout):
		require.NoError(t, cmd.Process.Kill())
		_, _ = cmd.Process.Wait()
		t.Fatal("helper process did not acquire the lock")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = New(path).WithExclusive(ctx, func() error { return errors.New("must not run") })
	require.ErrorIs(t, err, ErrAcquire)
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		require.NoError(t, err)
	case <-time.After(processWaitTimeout):
		require.NoError(t, cmd.Process.Kill())
		<-wait
		t.Fatal("helper process did not release the lock")
	}
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
