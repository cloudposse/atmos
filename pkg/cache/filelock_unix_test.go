//go:build !windows

package cache

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNewFileLock_LockPathSuffix(t *testing.T) {
	lock := NewFileLock("/some/path/cache")

	// Verify it returns a flockFileLock with the .lock extension.
	flockLock, ok := lock.(*flockFileLock)
	require.True(t, ok, "should return a flockFileLock")
	assert.Equal(t, "/some/path/cache.lock", flockLock.lockPath)
}

func TestWithLock_Success(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test-lock")
	lock := NewFileLock(lockPath)

	executed := false
	err := lock.WithLock(func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, executed, "function should have been executed")
}

func TestWithLock_FnError(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test-lock")
	lock := NewFileLock(lockPath)

	expectedErr := errors.New("test error")
	err := lock.WithLock(func() error {
		return expectedErr
	})

	require.Error(t, err)
	assert.Equal(t, expectedErr, err, "should return the function's error")
}

func TestWithRLock_Success(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test-lock")
	lock := NewFileLock(lockPath)

	executed := false
	err := lock.WithRLock(func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, executed, "function should have been executed")
}

func TestWithRLock_FnError(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test-lock")
	lock := NewFileLock(lockPath)

	expectedErr := errors.New("test error")
	err := lock.WithRLock(func() error {
		return expectedErr
	})

	require.Error(t, err)
	assert.Equal(t, expectedErr, err, "should return the function's error")
}

func TestWithLock_NestedCalls(t *testing.T) {
	// Test that a goroutine can acquire and release a lock successfully.
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "test-lock")
	lock := NewFileLock(lockPath)

	var outerExecuted, innerExecuted bool
	err := lock.WithLock(func() error {
		outerExecuted = true
		// The inner lock should succeed since flock is per-process on the same fd.
		return nil
	})

	require.NoError(t, err)
	assert.True(t, outerExecuted)

	// Now acquire the lock again (should work since previous lock is released).
	err = lock.WithLock(func() error {
		innerExecuted = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, innerExecuted)
}

func TestWithLock_RetryExhaustion(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "contended.lock")

	// Hold an exclusive lock from another flock instance.
	blocker := flock.New(lockPath)
	locked, err := blocker.TryLock()
	require.NoError(t, err)
	require.True(t, locked, "blocker should acquire lock")
	defer func() { _ = blocker.Unlock() }()

	// Now try to acquire the same lock through our FileLock.
	// It should exhaust retries and return ErrCacheLocked.
	lock := &flockFileLock{lockPath: lockPath}
	err = lock.WithLock(func() error {
		t.Fatal("function should not have been executed")
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCacheLocked)
}

func TestWithRLock_FallbackWithoutLock(t *testing.T) {
	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "contended.lock")

	// Hold an exclusive lock so TryRLock fails.
	blocker := flock.New(lockPath)
	locked, err := blocker.TryLock()
	require.NoError(t, err)
	require.True(t, locked, "blocker should acquire lock")
	defer func() { _ = blocker.Unlock() }()

	// WithRLock should still execute the function without the lock (line 88 fallback).
	lock := &flockFileLock{lockPath: lockPath}
	executed := false
	err = lock.WithRLock(func() error {
		executed = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, executed, "function should be executed without lock as fallback")
}

func TestWithLock_InvalidLockPath(t *testing.T) {
	// Use a path under a non-existent directory.
	lock := &flockFileLock{lockPath: "/nonexistent/dir/test.lock"}

	err := lock.WithLock(func() error {
		t.Fatal("function should not have been executed")
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCacheLocked)
}

func TestWithRLock_InvalidLockPath(t *testing.T) {
	// Use a path under a non-existent directory.
	lock := &flockFileLock{lockPath: "/nonexistent/dir/test.lock"}

	err := lock.WithRLock(func() error {
		t.Fatal("function should not have been executed")
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCacheLocked)
}
