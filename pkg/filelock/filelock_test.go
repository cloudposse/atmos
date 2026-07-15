package filelock

import (
	"bufio"
	"context"
	"errors"
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
	go func() {
		_ = New(path).WithExclusive(context.Background(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := New(path).WithExclusive(ctx, func() error { t.Fatal("lock must not be acquired"); return nil })
	require.ErrorIs(t, err, ErrAcquire)

	close(release)
	require.Eventually(t, func() bool {
		return New(path).WithExclusive(context.Background(), func() error { return nil }) == nil
	}, time.Second, 10*time.Millisecond)
}

func TestSharedLocksCanOverlapButBlockExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	firstHeld := make(chan struct{})
	secondHeld := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_ = New(path).WithShared(context.Background(), func() error {
			close(firstHeld)
			<-release
			return nil
		})
	}()
	<-firstHeld

	go func() {
		_ = New(path).WithShared(context.Background(), func() error {
			close(secondHeld)
			<-release
			return nil
		})
	}()
	select {
	case <-secondHeld:
	case <-time.After(time.Second):
		t.Fatal("shared locks should not block one another")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := New(path).WithExclusive(ctx, func() error {
		return errors.New("exclusive lock must not be acquired")
	})
	require.ErrorIs(t, err, ErrAcquire)

	close(release)
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

func TestExclusiveLockContendsAcrossProcesses(t *testing.T) {
	if os.Getenv("ATMOS_FILELOCK_HELPER") == "1" {
		path := os.Getenv("ATMOS_FILELOCK_PATH")
		_ = New(path).WithExclusive(context.Background(), func() error {
			_, _ = os.Stdout.WriteString("locked\n")
			time.Sleep(300 * time.Millisecond)
			return nil
		})
		return
	}

	path := filepath.Join(t.TempDir(), "cross-process.lock")
	cmd := exec.Command(os.Args[0], "-test.run=TestExclusiveLockContendsAcrossProcesses")
	cmd.Env = append(os.Environ(), "ATMOS_FILELOCK_HELPER=1", "ATMOS_FILELOCK_PATH="+path)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	scanner := bufio.NewScanner(stdout)
	require.True(t, scanner.Scan())
	require.Equal(t, "locked", scanner.Text())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = New(path).WithExclusive(ctx, func() error { return errors.New("must not run") })
	require.ErrorIs(t, err, ErrAcquire)
	require.NoError(t, cmd.Wait())
}
