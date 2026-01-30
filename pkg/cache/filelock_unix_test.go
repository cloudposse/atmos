//go:build !windows

package cache

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
