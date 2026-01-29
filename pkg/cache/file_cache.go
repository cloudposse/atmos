package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// DefaultCacheDirPerm is the default permission for cache directories.
	DefaultCacheDirPerm = 0o755
	// DefaultFilePerm is the default permission for cache files.
	DefaultFilePerm = 0o644
)

// FileCache provides atomic file-based caching with platform-specific locking.
// It stores cached content in an XDG-compliant cache directory.
type FileCache struct {
	baseDir string
	lock    FileLock
	fs      filesystem.FileSystem
}

// FileCacheOption is a functional option for configuring FileCache.
type FileCacheOption func(*FileCache)

// WithBaseDir sets a custom base directory for the cache.
// This is primarily useful for testing.
func WithBaseDir(dir string) FileCacheOption {
	defer perf.Track(nil, "cache.WithBaseDir")()

	return func(c *FileCache) {
		c.baseDir = dir
	}
}

// WithFileSystem sets a custom filesystem implementation.
// This is primarily useful for testing.
func WithFileSystem(fs filesystem.FileSystem) FileCacheOption {
	defer perf.Track(nil, "cache.WithFileSystem")()

	return func(c *FileCache) {
		c.fs = fs
	}
}

// NewFileCache creates a new FileCache in the specified XDG cache subdirectory.
// The subpath is relative to the XDG cache directory (e.g., "stack-imports" creates
// ~/.cache/atmos/stack-imports/).
func NewFileCache(subpath string, opts ...FileCacheOption) (*FileCache, error) {
	defer perf.Track(nil, "cache.NewFileCache")()

	// Get XDG cache directory.
	baseDir, err := xdg.GetXDGCacheDir(subpath, DefaultCacheDirPerm)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCacheDirectoryCreation).
			WithCause(err).
			WithContext("subpath", subpath).
			Err()
	}

	c := &FileCache{
		baseDir: baseDir,
		lock:    NewFileLock(filepath.Join(baseDir, "cache")),
		fs:      filesystem.NewOSFileSystem(),
	}

	// Apply options.
	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// keyToFilename converts a cache key to a filename using SHA256 hashing.
// This ensures valid filenames regardless of key content.
func keyToFilename(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:8])
}

// Get returns the cached content for a key.
// Returns (content, true, nil) if found, (nil, false, nil) if not found,
// or (nil, false, error) on read error.
func (c *FileCache) Get(key string) ([]byte, bool, error) {
	defer perf.Track(nil, "cache.FileCache.Get")()

	filename := keyToFilename(key)
	path := filepath.Join(c.baseDir, filename)

	var content []byte
	var exists bool

	err := c.lock.WithRLock(func() error {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				exists = false
				return nil
			}
			return readErr
		}
		content = data
		exists = true
		return nil
	})
	if err != nil {
		return nil, false, errUtils.Build(errUtils.ErrCacheRead).
			WithCause(err).
			WithContext("key", key).
			Err()
	}

	return content, exists, nil
}

// Set stores content for a key atomically.
func (c *FileCache) Set(key string, content []byte) error {
	defer perf.Track(nil, "cache.FileCache.Set")()

	filename := keyToFilename(key)
	path := filepath.Join(c.baseDir, filename)

	return c.lock.WithLock(func() error {
		if err := c.fs.WriteFileAtomic(path, content, DefaultFilePerm); err != nil {
			return errUtils.Build(errUtils.ErrCacheWrite).
				WithCause(err).
				WithContext("key", key).
				Err()
		}
		return nil
	})
}

// GetPath returns the filesystem path for a cached key.
// Returns (path, true) if the key exists in cache, (path, false) otherwise.
// This is useful when callers need the file path rather than content.
func (c *FileCache) GetPath(key string) (string, bool) {
	defer perf.Track(nil, "cache.FileCache.GetPath")()

	filename := keyToFilename(key)
	path := filepath.Join(c.baseDir, filename)

	if _, err := os.Stat(path); err == nil {
		return path, true
	}
	return path, false
}

// GetOrFetch returns cached content for a key, or calls fetch() and caches the result.
// This provides a convenient way to implement cache-aside patterns.
func (c *FileCache) GetOrFetch(key string, fetch func() ([]byte, error)) ([]byte, error) {
	defer perf.Track(nil, "cache.FileCache.GetOrFetch")()

	// Check cache first.
	content, exists, err := c.Get(key)
	if err != nil {
		return nil, err
	}
	if exists {
		return content, nil
	}

	// Fetch and cache.
	content, err = fetch()
	if err != nil {
		return nil, err
	}

	if err := c.Set(key, content); err != nil {
		// Log but don't fail - cache write errors are non-critical.
		// The content was fetched successfully.
		return content, nil
	}

	return content, nil
}

// Clear removes all cached entries from the cache directory.
func (c *FileCache) Clear() error {
	defer perf.Track(nil, "cache.FileCache.Clear")()

	return c.lock.WithLock(func() error {
		// Remove all files in the cache directory.
		entries, err := os.ReadDir(c.baseDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return errUtils.Build(errUtils.ErrClearCache).
				WithCause(err).
				WithContext("path", c.baseDir).
				Err()
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue // Skip subdirectories.
			}
			path := filepath.Join(c.baseDir, entry.Name())
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return errUtils.Build(errUtils.ErrClearCache).
					WithCause(err).
					WithContext("path", path).
					Err()
			}
		}

		return nil
	})
}

// BaseDir returns the base directory of the cache.
func (c *FileCache) BaseDir() string {
	defer perf.Track(nil, "cache.FileCache.BaseDir")()

	return c.baseDir
}
