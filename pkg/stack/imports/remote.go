package imports

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

const (
	// DefaultDownloadTimeout is the timeout for downloading remote imports.
	defaultDownloadTimeout = 60 * time.Second
)

var (
	errRemoteSubdirTraversal = errors.New("subdirectory component contains path traversal out of the repository")
	remoteStackExtensions    = []string{".yaml", ".yml", ".yaml.tmpl", ".yml.tmpl"}
)

// RemoteImportMatch is a resolved remote stack import ready for local processing.
type RemoteImportMatch struct {
	Path     string
	Key      string
	BasePath string
}

// RemoteImporter handles downloading stack imports from remote URLs.
type RemoteImporter struct {
	atmosConfig *schema.AtmosConfiguration
	downloader  downloader.FileDownloader
	cache       *cache.FileCache
	memCache    map[string]string // In-memory cache for session.
	matchCache  map[string][]RemoteImportMatch
	memMu       sync.RWMutex
	sourceMu    sync.Mutex
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
		atmosConfig: atmosConfig,
		downloader:  fd,
		cache:       fileCache,
		memCache:    make(map[string]string),
		matchCache:  make(map[string][]RemoteImportMatch),
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

// Resolve fetches a remote import and returns all local stack files it resolves to.
func (r *RemoteImporter) Resolve(uri string) ([]RemoteImportMatch, error) {
	defer perf.Track(nil, "imports.RemoteImporter.Resolve")()

	if !IsRemote(uri) {
		return nil, errUtils.Build(errUtils.ErrInvalidRemoteImport).
			WithExplanation("URI is not a remote URL").
			WithContext("uri", uri).
			Err()
	}

	if matches, ok := r.cachedMatches(uri); ok {
		return matches, nil
	}

	if !IsGitURI(uri) {
		path, err := r.Download(uri)
		if err != nil {
			return nil, err
		}
		matches := []RemoteImportMatch{{Path: path, Key: uri}}
		r.storeMatches(uri, matches)
		return matches, nil
	}

	sourceURI, subdir := getter.SourceDirSubdir(uri)
	if subdir == "" {
		path, err := r.Download(uri)
		if err != nil {
			return nil, err
		}
		matches := []RemoteImportMatch{{Path: path, Key: uri}}
		r.storeMatches(uri, matches)
		return matches, nil
	}

	matches, err := r.resolveGitSubdir(uri, sourceURI, subdir)
	if err != nil {
		return nil, err
	}
	r.storeMatches(uri, matches)
	return matches, nil
}

func (r *RemoteImporter) cachedMatches(uri string) ([]RemoteImportMatch, bool) {
	r.memMu.RLock()
	cached, ok := r.matchCache[uri]
	r.memMu.RUnlock()
	if !ok {
		return nil, false
	}
	for _, match := range cached {
		if _, err := os.Stat(match.Path); err == nil {
			continue
		}
		r.memMu.Lock()
		delete(r.matchCache, uri)
		for _, stale := range cached {
			delete(r.memCache, stale.Key)
		}
		r.memMu.Unlock()
		return nil, false
	}
	return cloneMatches(cached), true
}

func (r *RemoteImporter) storeMatches(uri string, matches []RemoteImportMatch) {
	r.memMu.Lock()
	defer r.memMu.Unlock()

	r.matchCache[uri] = cloneMatches(matches)
}

func cloneMatches(matches []RemoteImportMatch) []RemoteImportMatch {
	cloned := make([]RemoteImportMatch, len(matches))
	copy(cloned, matches)
	return cloned
}

func (r *RemoteImporter) resolveGitSubdir(originalURI, sourceURI, subdir string) ([]RemoteImportMatch, error) {
	sourceURI = r.detectGitSource(sourceURI)
	tempDir, err := os.MkdirTemp(r.cache.BaseDir(), uriToTempName(originalURI)+".dir-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	if err := r.downloader.Fetch(sourceURI, tempDir, downloader.ClientModeDir, defaultDownloadTimeout); err != nil {
		return nil, err
	}

	files, err := resolveStackFiles(tempDir, subdir)
	if err != nil {
		return nil, err
	}

	matches := make([]RemoteImportMatch, 0, len(files))
	for _, file := range files {
		rel, err := filepath.Rel(tempDir, file)
		if err != nil {
			return nil, err
		}
		key := originalURI + "#" + filepath.ToSlash(rel)
		cachedPath, err := r.cacheFile(key, file)
		if err != nil {
			return nil, err
		}
		matches = append(matches, RemoteImportMatch{Path: cachedPath, Key: key})
	}

	return matches, nil
}

func (r *RemoteImporter) detectGitSource(sourceURI string) string {
	atmosConfig := r.atmosConfig
	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{}
	}
	detector := downloader.NewCustomGitDetector(atmosConfig, sourceURI)
	detected, ok, err := detector.Detect(sourceURI, "")
	if err != nil || !ok {
		return sourceURI
	}
	return detected
}

func (r *RemoteImporter) cacheFile(key, sourcePath string) (string, error) {
	r.memMu.RLock()
	if cachedPath, ok := r.memCache[key]; ok {
		r.memMu.RUnlock()
		if _, err := os.Stat(cachedPath); err == nil {
			return cachedPath, nil
		}
		r.memMu.Lock()
		delete(r.memCache, key)
		r.memMu.Unlock()
	} else {
		r.memMu.RUnlock()
	}

	_, err := r.cache.GetOrFetch(key, func() ([]byte, error) {
		return os.ReadFile(sourcePath)
	})
	if err != nil {
		return "", err
	}

	destPath, _ := r.cache.GetPath(key)
	r.memMu.Lock()
	r.memCache[key] = destPath
	r.memMu.Unlock()
	return destPath, nil
}

func resolveStackFiles(root, subdir string) ([]string, error) {
	if subdir == "" {
		return nil, fmt.Errorf("%w: empty git subdirectory", errUtils.ErrFailedToFindImport)
	}

	if hasGlobMeta(subdir) {
		return resolveGlob(root, subdir)
	}

	cleanSubdir, err := cleanRemoteSubdir(subdir)
	if err != nil {
		return nil, err
	}

	target := filepath.Join(root, filepath.FromSlash(cleanSubdir))
	if filepath.Ext(cleanSubdir) == "" {
		for _, ext := range remoteStackExtensions {
			candidate := target + ext
			if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
				return []string{candidate}, nil
			}
		}
	}

	if info, statErr := os.Stat(target); statErr == nil {
		if info.IsDir() {
			return resolveDirectory(root, target)
		}
		return []string{target}, nil
	}

	return nil, errUtils.Build(errUtils.ErrFailedToFindImport).
		WithContext("pattern", filepath.ToSlash(target)).
		Err()
}

func resolveDirectory(root, dir string) ([]string, error) {
	patterns := make([]string, 0, len(remoteStackExtensions))
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	for _, ext := range remoteStackExtensions {
		patterns = append(patterns, filepath.ToSlash(filepath.Join(rel, "**", "*"+ext)))
	}
	return resolveGlobPatterns(root, patterns)
}

func resolveGlob(root, pattern string) ([]string, error) {
	cleanPattern, err := cleanRemoteSubdir(pattern)
	if err != nil {
		return nil, err
	}
	return resolveGlobPatterns(root, []string{cleanPattern})
}

func resolveGlobPatterns(root string, patterns []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, pattern := range patterns {
		matches, err := doublestar.Glob(os.DirFS(root), filepath.ToSlash(pattern))
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			fullPath := filepath.Join(root, filepath.FromSlash(match))
			info, err := os.Stat(fullPath)
			if err != nil || info.IsDir() {
				continue
			}
			rel, err := filepath.Rel(root, fullPath)
			if err != nil {
				return nil, err
			}
			normalized := filepath.ToSlash(rel)
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			files = append(files, fullPath)
		}
	}

	if len(files) == 0 {
		return nil, errUtils.Build(errUtils.ErrFailedToFindImport).
			WithContext("pattern", strings.Join(patterns, ",")).
			Err()
	}

	sort.Slice(files, func(i, j int) bool {
		left, _ := filepath.Rel(root, files[i])
		right, _ := filepath.Rel(root, files[j])
		return filepath.ToSlash(left) < filepath.ToSlash(right)
	})

	return files, nil
}

func cleanRemoteSubdir(subdir string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimPrefix(subdir, "/"))))
	if cleaned == "." || cleaned == "" {
		return ".", nil
	}
	if strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", errRemoteSubdirTraversal
	}
	return cleaned, nil
}

func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

// ClearCache removes all cached imports.
func (r *RemoteImporter) ClearCache() error {
	defer perf.Track(nil, "imports.RemoteImporter.ClearCache")()

	r.memMu.Lock()
	defer r.memMu.Unlock()

	// Clear in-memory cache.
	r.memCache = make(map[string]string)
	r.matchCache = make(map[string][]RemoteImportMatch)

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

// ResolveRemoteImport is a convenience function that uses the global importer.
func ResolveRemoteImport(atmosConfig *schema.AtmosConfiguration, uri string) ([]RemoteImportMatch, error) {
	defer perf.Track(atmosConfig, "imports.ResolveRemoteImport")()

	importer, err := getGlobalImporter(atmosConfig)
	if err != nil {
		return nil, err
	}
	return importer.Resolve(uri)
}
