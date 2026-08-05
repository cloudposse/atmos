//go:build !windows

package workdir

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Tests for withMetadataFileLockUnix function.

func TestWithMetadataFileLockUnix_Success(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")

	called := false
	err := withMetadataFileLockUnix(metadataFile, func() error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestWithMetadataFileLockUnix_FunctionError(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")

	err := withMetadataFileLockUnix(metadataFile, func() error {
		return errUtils.Build(errUtils.ErrWorkdirMetadata).
			WithExplanation("test error").
			Err()
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

func TestWithMetadataFileLockUnix_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")

	var wg sync.WaitGroup
	counter := 0
	var mu sync.Mutex

	// Run multiple goroutines that try to acquire the lock.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withMetadataFileLockUnix(metadataFile, func() error {
				mu.Lock()
				counter++
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				return nil
			})
			// Some may succeed, some may fail due to lock contention - both are OK.
			_ = err
		}()
	}

	wg.Wait()
	assert.GreaterOrEqual(t, counter, 1, "at least one goroutine should succeed")
}

// Tests for loadMetadataWithReadLockUnix function.

func TestLoadMetadataWithReadLockUnix_Success(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")

	// Write valid metadata.
	metadataJSON := `{
		"component": "vpc",
		"stack": "dev",
		"source_type": "local",
		"source": "components/terraform/vpc",
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-01T00:00:00Z"
	}`
	require.NoError(t, os.WriteFile(metadataFile, []byte(metadataJSON), 0o644))

	metadata, err := loadMetadataWithReadLockUnix(metadataFile)
	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, "vpc", metadata.Component)
	assert.Equal(t, "dev", metadata.Stack)
}

func TestLoadMetadataWithReadLockUnix_FileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "nonexistent.json")

	metadata, err := loadMetadataWithReadLockUnix(metadataFile)
	assert.NoError(t, err)
	assert.Nil(t, metadata)
}

func TestLoadMetadataWithReadLockUnix_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON.
	require.NoError(t, os.WriteFile(metadataFile, []byte("not valid json"), 0o644))

	metadata, err := loadMetadataWithReadLockUnix(metadataFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
	assert.Equal(t, 1, strings.Count(err.Error(), errUtils.ErrWorkdirMetadata.Error()))
}

func TestLoadMetadataWithReadLockUnix_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")

	// Create a directory instead of a file to cause read error.
	require.NoError(t, os.MkdirAll(metadataFile, 0o755))

	metadata, err := loadMetadataWithReadLockUnix(metadataFile)
	assert.Error(t, err)
	assert.Nil(t, metadata)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
	assert.Equal(t, 1, strings.Count(err.Error(), errUtils.ErrWorkdirMetadata.Error()))
}

func TestLoadMetadataWithReadLockUnix_ConcurrentReads(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")

	// Write valid metadata.
	metadataJSON := `{
		"component": "vpc",
		"stack": "dev",
		"source_type": "local",
		"source": "components/terraform/vpc",
		"created_at": "2024-01-01T00:00:00Z",
		"updated_at": "2024-01-01T00:00:00Z"
	}`
	require.NoError(t, os.WriteFile(metadataFile, []byte(metadataJSON), 0o644))

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Multiple concurrent reads should work.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			metadata, err := loadMetadataWithReadLockUnix(metadataFile)
			if err == nil && metadata != nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	assert.GreaterOrEqual(t, successCount, 1, "at least one read should succeed")
}

// TestWithMetadataFileLockUnix_LockUnavailable exercises the lock-failure branch of withMetadataFileLockUnix.
// The gofrs/flock library opens lock files with O_CREATE|O_RDONLY rather than O_RDWR, so occupying the lock path with a directory does not fail: open(2) on a directory with O_RDONLY succeeds, and flock(2) on a directory fd succeeds too, so that variant does not force a failure here.
// Instead, like pkg/cache/filelock_unix_test.go's TestWithLock_InvalidLockPath, this test points the metadata file at a path whose parent directory does not exist: O_CREATE then fails immediately with ENOENT, so the lock is never acquired and the 500ms context timeout never needs to elapse.
func TestWithMetadataFileLockUnix_LockUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "missing-dir", "metadata.json")

	called := false
	start := time.Now()
	err := withMetadataFileLockUnix(metadataFile, func() error {
		called = true
		return nil
	})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
	assert.False(t, called, "fn must not run when the lock cannot be acquired")
	assert.Less(t, elapsed, 400*time.Millisecond, "should fail fast without waiting on the 500ms timeout")
}

// TestLoadMetadataWithReadLockUnix_LockUnavailable exercises the
// "!acquired && err != nil" branch of loadMetadataWithReadLockUnix. That
// branch is distinct from the "!acquired && err == nil" contended-without-
// error path already exercised by TestLoadMetadataWithReadLockUnix_ConcurrentReads.
// As in TestWithMetadataFileLockUnix_LockUnavailable above, a directory at
// the lock path does not force an error here (gofrs/flock opens read-only,
// which succeeds against a directory). Instead, point the metadata file at a
// path whose parent directory does not exist, so the open() call inside
// TryWithRLock fails immediately with ENOENT (a hard error) rather than the
// 1ms internal timeout merely expiring without an error - deterministically
// forcing the hard-error variant.
func TestLoadMetadataWithReadLockUnix_LockUnavailable(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "missing-dir", "metadata.json")

	metadata, err := loadMetadataWithReadLockUnix(metadataFile)

	require.Error(t, err)
	assert.Nil(t, metadata)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

// TestLoadMetadataWithReadLockUnix_ContendedNoError exercises the
// "!acquired && err == nil" branch (the "return nil, nil" contended-read
// path). This is genuinely distinct from both LockUnavailable tests above:
// here the lock file opens fine but is already held exclusively by another
// holder, so TryWithRLock's 1ms internal timeout expires without an OS-level
// error. The existing goroutine-based TestLoadMetadataWithReadLockUnix_ConcurrentReads
// doesn't reliably hit this branch because shared (read) locks don't
// contend with each other, so this test forces it deterministically by
// holding a separate exclusive lock, matching the style of
// pkg/cache/filelock_unix_test.go's TestTryWithRLock_SkipsContendedRead.
func TestLoadMetadataWithReadLockUnix_ContendedNoError(t *testing.T) {
	tmpDir := t.TempDir()
	metadataFile := filepath.Join(tmpDir, "metadata.json")
	require.NoError(t, os.WriteFile(metadataFile, []byte(`{}`), 0o644))

	blocker := flock.New(metadataFile + ".lock")
	locked, err := blocker.TryLock()
	require.NoError(t, err)
	require.True(t, locked, "blocker should acquire the exclusive lock")
	defer func() { _ = blocker.Unlock() }()

	metadata, err := loadMetadataWithReadLockUnix(metadataFile)

	require.NoError(t, err)
	assert.Nil(t, metadata)
}
