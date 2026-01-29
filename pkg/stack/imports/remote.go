package imports

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultDownloadTimeout is the timeout for downloading remote imports.
	defaultDownloadTimeout = 60 * time.Second
)

// RemoteImporter handles downloading stack imports from remote URLs.
type RemoteImporter struct {
	downloader downloader.FileDownloader
	cache      *cache.FileCache
	memCache   map[string]string // In-memory cache for session.
	memMu      sync.RWMutex
}

// RemoteImporterOption is a functional option for configuring RemoteImporter.
type RemoteImporterOption func(*RemoteImporter)

// WithCache sets a custom FileCache (useful for testing).
func WithCache(c *cache.FileCache) RemoteImporterOption {
	defer perf.Track(nil, "imports.WithCache")()

	return func(r *RemoteImporter) {
		r.cache = c
	}
}

// WithDownloader sets a custom downloader (useful for testing).
func WithDownloader(d downloader.FileDownloader) RemoteImporterOption {
	defer perf.Track(nil, "imports.WithDownloader")()

	return func(r *RemoteImporter) {
		r.downloader = d
	}
}

// NewRemoteImporter creates a new RemoteImporter.
func NewRemoteImporter(atmosConfig *schema.AtmosConfiguration, opts ...RemoteImporterOption) (*RemoteImporter, error) {
	defer perf.Track(atmosConfig, "imports.NewRemoteImporter")()

	// Create default cache using XDG Base Directory Specification.
	fileCache, err := cache.NewFileCache("stack-imports")
	if err != nil {
		return nil, err
	}

	// Create default downloader using go-getter.
	fd := downloader.NewGoGetterDownloader(atmosConfig)

	r := &RemoteImporter{
		downloader: fd,
		cache:      fileCache,
		memCache:   make(map[string]string),
	}

	// Apply options.
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

// uriToTempName generates a unique temp filename for a URI using SHA256 hashing.
func uriToTempName(uri string) string {
	hash := sha256.Sum256([]byte(uri))
	return fmt.Sprintf(".download.%x", hash[:8])
}

// Download fetches a remote import and returns the local path.
// Downloads are cached by URL hash to avoid redundant downloads.
// Uses file locking and atomic writes for safe concurrent access.
func (r *RemoteImporter) Download(uri string) (string, error) {
	defer perf.Track(nil, "imports.RemoteImporter.Download")()

	if !IsRemote(uri) {
		return "", errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("URI is not a remote URL").
			WithContext("uri", uri).
			Err()
	}

	// Check in-memory cache first (session-level cache).
	r.memMu.RLock()
	if cachedPath, ok := r.memCache[uri]; ok {
		r.memMu.RUnlock()
		// Verify the file still exists.
		if _, err := os.Stat(cachedPath); err == nil {
			return cachedPath, nil
		}
		// File was deleted, remove from memory cache and re-download.
		r.memMu.Lock()
		delete(r.memCache, uri)
		r.memMu.Unlock()
	} else {
		r.memMu.RUnlock()
	}

	// Use GetOrFetch to properly handle file locking and atomic writes.
	// This ensures safe concurrent access across multiple atmos processes.
	_, err := r.cache.GetOrFetch(uri, func() ([]byte, error) {
		// Download to a temporary location first, then read the content.
		// The cache will handle atomic writes and file locking.
		tempPath := filepath.Join(r.cache.BaseDir(), uriToTempName(uri))
		defer os.Remove(tempPath)

		if fetchErr := r.downloader.Fetch(uri, tempPath, downloader.ClientModeFile, defaultDownloadTimeout); fetchErr != nil {
			return nil, fetchErr
		}

		return os.ReadFile(tempPath)
	})
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDownloadRemoteImport).
			WithCause(err).
			WithContext("uri", uri).
			WithHint("Check network connectivity and verify the URL is accessible").
			Err()
	}

	// Get the final cached path.
	destPath, _ := r.cache.GetPath(uri)

	// Add to memory cache.
	r.memMu.Lock()
	r.memCache[uri] = destPath
	r.memMu.Unlock()

	return destPath, nil
}

// ClearCache removes all cached imports.
func (r *RemoteImporter) ClearCache() error {
	defer perf.Track(nil, "imports.RemoteImporter.ClearCache")()

	r.memMu.Lock()
	defer r.memMu.Unlock()

	// Clear in-memory cache.
	r.memCache = make(map[string]string)

	// Clear persistent cache.
	return r.cache.Clear()
}

// global remote importer instance (lazily initialized).
var (
	globalImporter     *RemoteImporter
	globalImporterOnce sync.Once
	globalImporterErr  error
)

// getGlobalImporter returns the global RemoteImporter instance.
func getGlobalImporter(atmosConfig *schema.AtmosConfiguration) (*RemoteImporter, error) {
	globalImporterOnce.Do(func() {
		globalImporter, globalImporterErr = NewRemoteImporter(atmosConfig)
	})
	return globalImporter, globalImporterErr
}

// DownloadRemoteImport is a convenience function that uses the global importer.
func DownloadRemoteImport(atmosConfig *schema.AtmosConfiguration, uri string) (string, error) {
	defer perf.Track(atmosConfig, "imports.DownloadRemoteImport")()

	importer, err := getGlobalImporter(atmosConfig)
	if err != nil {
		return "", err
	}
	return importer.Download(uri)
}
