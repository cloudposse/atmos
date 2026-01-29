package imports

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// defaultDownloadTimeout is the timeout for downloading remote imports.
	defaultDownloadTimeout = 60 * time.Second
)

// RemoteImporter handles downloading stack imports from remote URLs.
type RemoteImporter struct {
	downloader downloader.FileDownloader
	cacheDir   string
	cache      map[string]string
	cacheMu    sync.RWMutex
}

// RemoteImporterOption is a functional option for configuring RemoteImporter.
type RemoteImporterOption func(*RemoteImporter)

// WithCacheDir sets the cache directory for downloaded imports.
func WithCacheDir(dir string) RemoteImporterOption {
	return func(r *RemoteImporter) {
		r.cacheDir = dir
	}
}

// WithDownloader sets a custom downloader (useful for testing).
func WithDownloader(d downloader.FileDownloader) RemoteImporterOption {
	return func(r *RemoteImporter) {
		r.downloader = d
	}
}

// NewRemoteImporter creates a new RemoteImporter.
func NewRemoteImporter(atmosConfig *schema.AtmosConfiguration, opts ...RemoteImporterOption) (*RemoteImporter, error) {
	defer perf.Track(atmosConfig, "imports.NewRemoteImporter")()

	// Create default cache directory.
	cacheDir := filepath.Join(os.TempDir(), "atmos-stack-imports")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, errUtils.Build(errUtils.ErrCacheDirectoryCreation).
			WithCause(err).
			WithContext("path", cacheDir).
			Err()
	}

	// Create default downloader using go-getter.
	fd := downloader.NewGoGetterDownloader(atmosConfig)

	r := &RemoteImporter{
		downloader: fd,
		cacheDir:   cacheDir,
		cache:      make(map[string]string),
	}

	// Apply options.
	for _, opt := range opts {
		opt(r)
	}

	return r, nil
}

// Download fetches a remote import and returns the local path.
// Downloads are cached by URL hash to avoid redundant downloads.
func (r *RemoteImporter) Download(uri string) (string, error) {
	defer perf.Track(nil, "imports.RemoteImporter.Download")()

	if !IsRemote(uri) {
		return "", errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("URI is not a remote URL").
			WithContext("uri", uri).
			Err()
	}

	// Check in-memory cache first.
	r.cacheMu.RLock()
	if cachedPath, ok := r.cache[uri]; ok {
		r.cacheMu.RUnlock()
		// Verify the file still exists.
		if _, err := os.Stat(cachedPath); err == nil {
			return cachedPath, nil
		}
		// File was deleted, remove from cache and re-download.
		r.cacheMu.Lock()
		delete(r.cache, uri)
		r.cacheMu.Unlock()
	} else {
		r.cacheMu.RUnlock()
	}

	// Generate cache filename from URI hash.
	hash := sha256.Sum256([]byte(uri))
	cacheFile := fmt.Sprintf("%x.yaml", hash[:8])
	destPath := filepath.Join(r.cacheDir, cacheFile)

	// Check if file already exists on disk (from previous run).
	if _, err := os.Stat(destPath); err == nil {
		r.cacheMu.Lock()
		r.cache[uri] = destPath
		r.cacheMu.Unlock()
		return destPath, nil
	}

	// Download the file.
	if err := r.downloader.Fetch(uri, destPath, downloader.ClientModeFile, defaultDownloadTimeout); err != nil {
		return "", errUtils.Build(errUtils.ErrDownloadRemoteImport).
			WithCause(err).
			WithContext("uri", uri).
			WithHint("Check network connectivity and verify the URL is accessible").
			Err()
	}

	// Add to cache.
	r.cacheMu.Lock()
	r.cache[uri] = destPath
	r.cacheMu.Unlock()

	return destPath, nil
}

// ClearCache removes all cached imports.
func (r *RemoteImporter) ClearCache() error {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// Clear in-memory cache.
	r.cache = make(map[string]string)

	// Clear disk cache.
	if err := os.RemoveAll(r.cacheDir); err != nil {
		return errUtils.Build(errUtils.ErrClearCache).
			WithCause(err).
			WithContext("path", r.cacheDir).
			Err()
	}

	// Recreate cache directory.
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return errUtils.Build(errUtils.ErrCacheDirectoryCreation).
			WithCause(err).
			WithContext("path", r.cacheDir).
			Err()
	}

	return nil
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
	importer, err := getGlobalImporter(atmosConfig)
	if err != nil {
		return "", err
	}
	return importer.Download(uri)
}
