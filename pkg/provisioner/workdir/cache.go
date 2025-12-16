package workdir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

// DefaultCache is the default implementation of the Cache interface.
// It uses XDG cache directories for storing downloaded component sources.
type DefaultCache struct {
	basePath string
	mu       sync.RWMutex
	index    *cacheIndex
}

// cacheIndex tracks all cache entries.
type cacheIndex struct {
	Entries map[string]*CacheEntry `json:"entries"`
}

// NewDefaultCache creates a new default cache implementation.
func NewDefaultCache() *DefaultCache {
	defer perf.Track(nil, "workdir.NewDefaultCache")()

	return &DefaultCache{
		index: &cacheIndex{
			Entries: make(map[string]*CacheEntry),
		},
	}
}

// ensureBasePath initializes the cache base path if not already set.
func (c *DefaultCache) ensureBasePath() error {
	if c.basePath != "" {
		return nil
	}

	path, err := xdg.GetXDGCacheDir(CacheDir, DirPermissions)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheRead).
			WithCause(err).
			WithExplanation("failed to get XDG cache directory").
			Err()
	}
	c.basePath = path

	// Load existing index if present.
	if err := c.loadIndex(); err != nil {
		// Not an error if index doesn't exist - start fresh.
		c.index = &cacheIndex{Entries: make(map[string]*CacheEntry)}
	}

	return nil
}

// loadIndex loads the cache index from disk.
func (c *DefaultCache) loadIndex() error {
	indexPath := filepath.Join(c.basePath, "index.json")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index cacheIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	c.index = &index
	return nil
}

// saveIndex saves the cache index to disk.
func (c *DefaultCache) saveIndex() error {
	indexPath := filepath.Join(c.basePath, "index.json")
	data, err := json.MarshalIndent(c.index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, FilePermissionsSecure)
}

// Get returns the cache entry for the given key, or nil if not found.
func (c *DefaultCache) Get(key string) (*CacheEntry, error) {
	defer perf.Track(nil, "workdir.DefaultCache.Get")()

	if err := c.ensureBasePath(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.index.Entries[key]
	if !ok {
		return nil, nil
	}

	// Check if entry has expired.
	if entry.TTL > 0 && time.Since(entry.CreatedAt) > entry.TTL {
		return nil, nil
	}

	// Verify content still exists.
	contentPath := c.contentPath(key)
	if _, err := os.Stat(contentPath); os.IsNotExist(err) {
		return nil, nil
	}

	// Update last accessed time.
	entry.LastAccessedAt = time.Now()

	return entry, nil
}

// Put stores the content from srcPath in the cache with the given key and metadata.
func (c *DefaultCache) Put(key string, srcPath string, entry *CacheEntry) error {
	defer perf.Track(nil, "workdir.DefaultCache.Put")()

	if err := c.ensureBasePath(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Create content directory.
	contentPath := c.contentPath(key)
	if err := os.MkdirAll(filepath.Dir(contentPath), DirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to create cache directory").
			Err()
	}

	// Copy content to cache.
	if err := copyDir(srcPath, contentPath); err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to copy content to cache").
			Err()
	}

	// Update entry with path.
	entry.Path = contentPath

	// Store in index.
	c.index.Entries[key] = entry

	// Save index.
	if err := c.saveIndex(); err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to save cache index").
			Err()
	}

	return nil
}

// Remove removes the cache entry for the given key.
func (c *DefaultCache) Remove(key string) error {
	defer perf.Track(nil, "workdir.DefaultCache.Remove")()

	if err := c.ensureBasePath(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove content.
	contentPath := c.contentPath(key)
	if err := os.RemoveAll(contentPath); err != nil && !os.IsNotExist(err) {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to remove cached content").
			Err()
	}

	// Remove from index.
	delete(c.index.Entries, key)

	// Save index.
	if err := c.saveIndex(); err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to save cache index").
			Err()
	}

	return nil
}

// Clear removes all cache entries.
func (c *DefaultCache) Clear() error {
	defer perf.Track(nil, "workdir.DefaultCache.Clear")()

	if err := c.ensureBasePath(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove the entire cache directory.
	if err := os.RemoveAll(c.basePath); err != nil {
		return errUtils.Build(errUtils.ErrSourceCacheWrite).
			WithCause(err).
			WithExplanation("failed to clear cache").
			Err()
	}

	// Reset index.
	c.index = &cacheIndex{Entries: make(map[string]*CacheEntry)}
	c.basePath = ""

	return nil
}

// Path returns the filesystem path for a cache key.
// Returns empty string if the entry doesn't exist.
func (c *DefaultCache) Path(key string) string {
	defer perf.Track(nil, "workdir.DefaultCache.Path")()

	if c.basePath == "" {
		if err := c.ensureBasePath(); err != nil {
			return ""
		}
	}

	entry, err := c.Get(key)
	if err != nil || entry == nil {
		return ""
	}

	return entry.Path
}

// GenerateKey generates a content-addressable cache key from a source config.
func (c *DefaultCache) GenerateKey(source *SourceConfig) string {
	defer perf.Track(nil, "workdir.DefaultCache.GenerateKey")()

	h := sha256.New()
	h.Write([]byte(normalizeURI(source.URI)))
	if source.Version != "" {
		h.Write([]byte(source.Version))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// GetPolicy determines the cache policy for a source config.
func (c *DefaultCache) GetPolicy(source *SourceConfig) CachePolicy {
	defer perf.Track(nil, "workdir.DefaultCache.GetPolicy")()

	// Check if version looks like a tag or commit SHA.
	if source.Version != "" {
		// Semantic version tags (v1.2.3, 1.2.3).
		if isSemver(source.Version) {
			return CachePolicyPermanent
		}
		// Commit SHAs (40 hex chars).
		if isCommitSHA(source.Version) {
			return CachePolicyPermanent
		}
	}

	// Check the URI for ref parameter.
	ref := extractRefFromURI(source.URI)
	if ref != "" {
		if isSemver(ref) {
			return CachePolicyPermanent
		}
		if isCommitSHA(ref) {
			return CachePolicyPermanent
		}
	}

	// Default to TTL-based for branches.
	return CachePolicyTTL
}

// contentPath returns the path to content for a cache key.
func (c *DefaultCache) contentPath(key string) string {
	// Use first 2 chars of hash as subdirectory for sharding.
	return filepath.Join(c.basePath, "blobs", key[:2], key, "content")
}

// normalizeURI normalizes a URI for consistent hashing.
func normalizeURI(uri string) string {
	defer perf.Track(nil, "workdir.normalizeURI")()

	// Trim whitespace.
	uri = strings.TrimSpace(uri)
	// Normalize to lowercase for scheme comparison.
	// Keep the rest as-is for case-sensitive paths.
	if idx := strings.Index(uri, "://"); idx != -1 {
		uri = strings.ToLower(uri[:idx]) + uri[idx:]
	}
	return uri
}

// isSemver checks if a string looks like a semantic version.
func isSemver(s string) bool {
	// Match v1.2.3, 1.2.3, v1.2.3-rc1, etc.
	re := regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)
	return re.MatchString(s)
}

// isCommitSHA checks if a string looks like a git commit SHA.
func isCommitSHA(s string) bool {
	// Full SHA is 40 hex chars, short SHA is 7+.
	re := regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	return re.MatchString(strings.ToLower(s))
}

// extractRefFromURI extracts the ref parameter from a URI.
func extractRefFromURI(uri string) string {
	defer perf.Track(nil, "workdir.extractRefFromURI")()

	// Look for ref= in query string.
	idx := strings.Index(uri, "ref=")
	if idx == -1 {
		return ""
	}

	ref := uri[idx+4:]
	// Find end of ref value.
	if end := strings.IndexAny(ref, "&?#"); end != -1 {
		ref = ref[:end]
	}

	return ref
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	defer perf.Track(nil, "workdir.copyDir")()

	// Get source info.
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory.
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory entries.
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	defer perf.Track(nil, "workdir.copyFile")()

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := srcFile.Read(buf)
		if n > 0 {
			if _, writeErr := dstFile.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return err
		}
	}

	return nil
}
