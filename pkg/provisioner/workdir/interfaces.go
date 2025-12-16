package workdir

import (
	"context"
	"io/fs"
)

//go:generate go run go.uber.org/mock/mockgen@latest -source=interfaces.go -destination=mock_interfaces_test.go -package=workdir

// Downloader downloads component sources from remote URIs.
// It abstracts the go-getter library for testability.
type Downloader interface {
	// Download downloads the source from the given URI to the destination path.
	// The URI should be a go-getter compatible URL.
	// Returns an error if the download fails.
	Download(ctx context.Context, uri, dest string) error
}

// FileSystem abstracts file system operations for testability.
type FileSystem interface {
	// MkdirAll creates a directory along with any necessary parents.
	MkdirAll(path string, perm fs.FileMode) error

	// RemoveAll removes path and any children it contains.
	RemoveAll(path string) error

	// Exists checks if a path exists.
	Exists(path string) bool

	// ReadFile reads the contents of a file.
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data to a file with the given permissions.
	WriteFile(path string, data []byte, perm fs.FileMode) error

	// CopyDir recursively copies a directory from src to dst.
	CopyDir(src, dst string) error

	// Walk walks the file tree rooted at root, calling fn for each file or directory.
	Walk(root string, fn fs.WalkDirFunc) error

	// Stat returns file info for the given path.
	Stat(path string) (fs.FileInfo, error)
}

// Cache manages the XDG component source cache.
type Cache interface {
	// Get returns the cache entry for the given key, or nil if not found.
	Get(key string) (*CacheEntry, error)

	// Put stores the content from srcPath in the cache with the given key and metadata.
	Put(key string, srcPath string, entry *CacheEntry) error

	// Remove removes the cache entry for the given key.
	Remove(key string) error

	// Clear removes all cache entries.
	Clear() error

	// Path returns the filesystem path for a cache key.
	// Returns empty string if the entry doesn't exist.
	Path(key string) string

	// GenerateKey generates a content-addressable cache key from a source config.
	GenerateKey(source *SourceConfig) string

	// GetPolicy determines the cache policy for a source config.
	GetPolicy(source *SourceConfig) CachePolicy
}

// Hasher computes content hashes for change detection.
type Hasher interface {
	// HashDir computes a hash of all files in a directory.
	HashDir(path string) (string, error)

	// HashFile computes a hash of a single file.
	HashFile(path string) (string, error)
}

// PathFilter filters paths based on include/exclude patterns.
type PathFilter interface {
	// Match returns true if the path should be included.
	Match(path string, includedPaths, excludedPaths []string) (bool, error)
}
