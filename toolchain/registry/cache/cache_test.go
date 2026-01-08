package cache

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFileStore(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := "/tmp/test-cache"
	store := NewFileStore(baseDir)

	assert.NotNil(t, store)
	assert.Equal(t, baseDir, store.baseDir)
}

func TestFileStore_SetAndGet(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	// Create temp directory for test.
	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	tests := []struct {
		name    string
		key     string
		data    []byte
		ttl     time.Duration
		wantErr bool
	}{
		{
			name:    "valid data with TTL",
			key:     "test-key",
			data:    []byte("test data"),
			ttl:     1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "empty data",
			key:     "empty-key",
			data:    []byte(""),
			ttl:     1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "very long TTL",
			key:     "long-ttl",
			data:    []byte("data"),
			ttl:     100 * time.Hour,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set data.
			err := store.Set(ctx, tt.key, tt.data, tt.ttl)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Get data back.
			got, err := store.Get(ctx, tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.data, got)
		})
	}
}

func TestFileStore_GetCacheMiss(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Try to get non-existent key.
	_, err := store.Get(ctx, "non-existent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestFileStore_GetExpired(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Set data with very short TTL.
	key := "expires-soon"
	data := []byte("test data")
	err := store.Set(ctx, key, data, 1*time.Millisecond)
	require.NoError(t, err)

	// Wait for expiration.
	time.Sleep(10 * time.Millisecond)

	// Try to get expired data.
	_, err = store.Get(ctx, key)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCacheExpired)
}

func TestFileStore_IsExpired(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	tests := []struct {
		name        string
		key         string
		ttl         time.Duration
		waitTime    time.Duration
		wantExpired bool
		wantErr     bool
	}{
		{
			name:        "not expired",
			key:         "fresh",
			ttl:         1 * time.Hour,
			waitTime:    0,
			wantExpired: false,
			wantErr:     false,
		},
		{
			name:        "expired",
			key:         "stale",
			ttl:         1 * time.Millisecond,
			waitTime:    10 * time.Millisecond,
			wantExpired: true,
			wantErr:     false,
		},
		{
			name:        "non-existent",
			key:         "missing",
			ttl:         0,
			waitTime:    0,
			wantExpired: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name != "non-existent" {
				// Set data.
				err := store.Set(ctx, tt.key, []byte("data"), tt.ttl)
				require.NoError(t, err)

				// Wait if needed.
				if tt.waitTime > 0 {
					time.Sleep(tt.waitTime)
				}
			}

			// Check expiration.
			expired, err := store.IsExpired(ctx, tt.key)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantExpired, expired)
			}
		})
	}
}

func TestFileStore_Delete(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Set data.
	key := "delete-me"
	err := store.Set(ctx, key, []byte("test data"), 1*time.Hour)
	require.NoError(t, err)

	// Verify it exists.
	_, err = store.Get(ctx, key)
	require.NoError(t, err)

	// Delete it.
	err = store.Delete(ctx, key)
	require.NoError(t, err)

	// Verify it's gone.
	_, err = store.Get(ctx, key)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestFileStore_Clear(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Set multiple entries.
	keys := []string{"key1", "key2", "key3"}
	for _, key := range keys {
		err := store.Set(ctx, key, []byte("data"), 1*time.Hour)
		require.NoError(t, err)
	}

	// Clear all.
	err := store.Clear(ctx)
	require.NoError(t, err)

	// Verify all are gone.
	for _, key := range keys {
		_, err := store.Get(ctx, key)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrCacheMiss)
	}
}

func TestFileStore_ConcurrentAccess(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Test concurrent writes and reads with unique keys.
	const goroutines = 10
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			key := fmt.Sprintf("concurrent-%d", id)
			data := []byte(fmt.Sprintf("data-%d", id))

			// Write.
			err := store.Set(ctx, key, data, 1*time.Hour)
			require.NoError(t, err)

			// Read.
			got, err := store.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, data, got)
		}(i)
	}

	// Wait for all goroutines.
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

func TestFileStore_LargeData(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Create large data (1MB).
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	key := "large-data"
	err := store.Set(ctx, key, largeData, 1*time.Hour)
	require.NoError(t, err)

	got, err := store.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, largeData, got)
}

func TestFileStore_PermissionError(t *testing.T) {
	defer func(t *testing.T) {
		t.Helper()
	}(t)

	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	// Skip on Windows - chmod doesn't work the same way on Windows.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows - chmod semantics differ")
	}

	baseDir := t.TempDir()
	store := NewFileStore(baseDir)
	ctx := context.Background()

	// Set data.
	key := "permission-test"
	err := store.Set(ctx, key, []byte("data"), 1*time.Hour)
	require.NoError(t, err)

	// Make the entire cache directory read-only to trigger permission error.
	err = os.Chmod(baseDir, 0o000)
	require.NoError(t, err)
	defer os.Chmod(baseDir, 0o755) // Cleanup.

	// Try to write - should fail with permission error.
	err = store.Set(ctx, "another-key", []byte("data"), 1*time.Hour)
	require.Error(t, err)
}
