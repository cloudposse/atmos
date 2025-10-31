package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Store defines the interface for cache operations.
type Store interface {
	// Get retrieves data from cache. Returns ErrCacheMiss if not found or expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores data in cache with the specified TTL.
	Set(ctx context.Context, key string, data []byte, ttl time.Duration) error

	// Delete removes an entry from cache.
	Delete(ctx context.Context, key string) error

	// Clear removes all entries from cache.
	Clear(ctx context.Context) error

	// IsExpired checks if a cache entry is expired without reading it.
	IsExpired(ctx context.Context, key string) (bool, error)
}

// Entry represents a cached entry with metadata.
type Entry struct {
	Data      []byte    `json:"data"`
	CachedAt  time.Time `json:"cached_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// FileStore implements file-based caching.
type FileStore struct {
	baseDir string
}

// NewFileStore creates a new file-based cache store.
func NewFileStore(baseDir string) *FileStore {
	defer perf.Track(nil, "cache.NewFileStore")()

	return &FileStore{
		baseDir: baseDir,
	}
}

// Get retrieves data from the file cache.
func (fs *FileStore) Get(ctx context.Context, key string) ([]byte, error) {
	defer perf.Track(nil, "cache.FileStore.Get")()

	cachePath := fs.getCachePath(key)

	// Read cache file.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	// Parse entry.
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Cache file corrupted, treat as miss.
		return nil, ErrCacheMiss
	}

	// Check expiration.
	if time.Now().After(entry.ExpiresAt) {
		return nil, ErrCacheExpired
	}

	return entry.Data, nil
}

// Set stores data in the file cache.
func (fs *FileStore) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	defer perf.Track(nil, "cache.FileStore.Set")()

	cachePath := fs.getCachePath(key)

	// Ensure cache directory exists.
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create entry.
	entry := Entry{
		Data:      data,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}

	// Marshal entry.
	entryData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// Write to file.
	//nolint:gosec // G306: Cache file needs to be readable by other processes/users
	if err := os.WriteFile(cachePath, entryData, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Delete removes an entry from the cache.
func (fs *FileStore) Delete(ctx context.Context, key string) error {
	defer perf.Track(nil, "cache.FileStore.Delete")()

	cachePath := fs.getCachePath(key)

	if err := os.Remove(cachePath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted.
		}
		return fmt.Errorf("failed to delete cache file: %w", err)
	}

	return nil
}

// Clear removes all entries from the cache.
func (fs *FileStore) Clear(ctx context.Context) error {
	defer perf.Track(nil, "cache.FileStore.Clear")()

	if err := os.RemoveAll(fs.baseDir); err != nil {
		return fmt.Errorf("failed to clear cache directory: %w", err)
	}

	return nil
}

// IsExpired checks if a cache entry is expired.
func (fs *FileStore) IsExpired(ctx context.Context, key string) (bool, error) {
	defer perf.Track(nil, "cache.FileStore.IsExpired")()

	cachePath := fs.getCachePath(key)

	// Check if file exists.
	stat, err := os.Stat(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil // Doesn't exist = expired.
		}
		return true, fmt.Errorf("failed to stat cache file: %w", err)
	}

	// Read entry to check expiration time.
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return true, fmt.Errorf("failed to read cache file: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return true, nil // Corrupted = expired.
	}

	// Use modification time as fallback if entry is invalid.
	if entry.ExpiresAt.IsZero() {
		// Default TTL: 24 hours from modification time.
		expiresAt := stat.ModTime().Add(24 * time.Hour)
		return time.Now().After(expiresAt), nil
	}

	return time.Now().After(entry.ExpiresAt), nil
}

// getCachePath returns the file path for a cache key.
func (fs *FileStore) getCachePath(key string) string {
	// Sanitize key to be filesystem-safe.
	safeKey := filepath.Clean(key)
	return filepath.Join(fs.baseDir, safeKey+".json")
}
