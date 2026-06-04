//go:build !windows

package workdir

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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
