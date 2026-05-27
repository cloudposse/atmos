package imports

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/cache"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

type stubRemoteDownloader struct {
	fetch func(src, dest string, mode downloader.ClientMode, timeout time.Duration) error
}

func (s stubRemoteDownloader) Fetch(src, dest string, mode downloader.ClientMode, timeout time.Duration) error {
	return s.fetch(src, dest, mode, timeout)
}

func (s stubRemoteDownloader) FetchAndAutoParse(string) (any, error) {
	return nil, errors.New("not implemented")
}

func (s stubRemoteDownloader) FetchAndParseByExtension(string) (any, error) {
	return nil, errors.New("not implemented")
}

func (s stubRemoteDownloader) FetchAndParseRaw(string) (any, error) {
	return nil, errors.New("not implemented")
}

func (s stubRemoteDownloader) FetchData(string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (s stubRemoteDownloader) FetchAtomic(string, string, downloader.ClientMode, time.Duration) error {
	return errors.New("not implemented")
}

// newTestRemoteImporter creates a RemoteImporter for testing with a temp cache directory.
func newTestRemoteImporter(t *testing.T, atmosConfig *schema.AtmosConfiguration) *RemoteImporter {
	t.Helper()
	tempDir := t.TempDir()

	// Create a FileCache with the temp directory.
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(tempDir))
	require.NoError(t, err)

	importer, err := NewRemoteImporter(atmosConfig, WithCache(testCache))
	require.NoError(t, err)

	return importer
}

func initGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "checkout", "-b", "main")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")

	for name, content := range files {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	runGit(t, repoDir, "add", ".")
	runGit(t, repoDir, "commit", "-m", "initial")
	return repoDir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func gitFileURI(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if filepath.VolumeName(path) != "" && cleaned != "" && cleaned[0] != '/' {
		cleaned = "/" + cleaned
	}
	return (&url.URL{Scheme: "file", Path: cleaned}).String()
}

func normalizeLineEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func TestRemoteImporter_Download_HTTP(t *testing.T) {
	// Create a mock HTTP server.
	content := `
components:
  terraform:
    vpc:
      vars:
        name: "test-vpc"
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Create RemoteImporter with test cache.
	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	// Download the file.
	localPath, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, localPath)

	// Verify the file exists and has correct content.
	data, err := os.ReadFile(localPath)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))

	// Download again - should hit cache.
	localPath2, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	assert.Equal(t, localPath, localPath2, "should return cached path")
}

func TestRemoteImporter_Resolve_HTTP_CachesClonesAndInvalidates(t *testing.T) {
	var downloadCount atomic.Int32
	content := "vars:\n  from_http: true\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadCount.Add(1)
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)
	uri := server.URL + "/config.yaml"

	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, uri, matches[0].Key)
	assert.NotEmpty(t, matches[0].Path)
	initialDownloadCount := downloadCount.Load()
	assert.Greater(t, initialDownloadCount, int32(0))

	data, err := os.ReadFile(matches[0].Path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))

	cachedPath := matches[0].Path
	matches[0].Key = "mutated"

	cachedMatches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, cachedMatches, 1)
	assert.Equal(t, uri, cachedMatches[0].Key, "cached matches should be cloned before storing")
	assert.Equal(t, cachedPath, cachedMatches[0].Path)
	assert.Equal(t, initialDownloadCount, downloadCount.Load(), "match cache should avoid re-downloading")

	cachedMatches[0].Key = "mutated-again"
	cachedMatchesAgain, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, cachedMatchesAgain, 1)
	assert.Equal(t, uri, cachedMatchesAgain[0].Key, "cached matches should be cloned before returning")

	require.NoError(t, os.Remove(cachedPath))

	refetchedMatches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, refetchedMatches, 1)
	assert.Equal(t, uri, refetchedMatches[0].Key)
	assert.Greater(t, downloadCount.Load(), initialDownloadCount, "missing cached file should invalidate the match cache")
}

func TestRemoteImporter_Resolve_LocalPathError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	matches, err := importer.Resolve("catalog/vpc.yaml")
	require.Error(t, err)
	assert.Nil(t, matches)
	assert.ErrorIs(t, err, errUtils.ErrInvalidRemoteImport)
}

func TestRemoteImporter_Resolve_GitDownloaderError(t *testing.T) {
	expectedErr := errors.New("download failed")
	testCache, err := cache.NewFileCache("test", cache.WithBaseDir(t.TempDir()))
	require.NoError(t, err)

	importer, err := NewRemoteImporter(
		&schema.AtmosConfiguration{},
		WithCache(testCache),
		WithDownloader(stubRemoteDownloader{
			fetch: func(_ string, _ string, mode downloader.ClientMode, _ time.Duration) error {
				assert.Equal(t, downloader.ClientModeDir, mode)
				return expectedErr
			},
		}),
	)
	require.NoError(t, err)

	matches, err := importer.Resolve("git::https://example.com/acme/infrastructure.git//stacks?ref=main")
	require.Error(t, err)
	assert.Nil(t, matches)
	assert.ErrorIs(t, err, expectedErr)
}

func TestRemoteImporter_Resolve_GitSubdirNoExtension(t *testing.T) {
	repoDir := initGitRepo(t, map[string]string{
		"stacks/orgs/acme/plat/dev.yaml": "vars:\n  imported: true\n",
	})

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	uri := fmt.Sprintf("git::%s//stacks/orgs/acme/plat/dev?ref=main", gitFileURI(repoDir))
	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, uri+"#stacks/orgs/acme/plat/dev.yaml", matches[0].Key)

	data, err := os.ReadFile(matches[0].Path)
	require.NoError(t, err)
	assert.Equal(t, "vars:\n  imported: true\n", normalizeLineEndings(string(data)))
}

func TestRemoteImporter_Resolve_GitDirectoryRecursive(t *testing.T) {
	repoDir := initGitRepo(t, map[string]string{
		"imports/b.yaml":            "vars:\n  order: b\n",
		"imports/nested/a.yaml":     "vars:\n  order: a\n",
		"imports/template.yml.tmpl": "vars:\n  template: true\n",
		"imports/ignored.txt":       "ignored\n",
	})

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	uri := fmt.Sprintf("git::%s//imports?ref=main", gitFileURI(repoDir))
	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 3)

	assert.Equal(t, []string{
		uri + "#imports/b.yaml",
		uri + "#imports/nested/a.yaml",
		uri + "#imports/template.yml.tmpl",
	}, []string{matches[0].Key, matches[1].Key, matches[2].Key})
}

func TestRemoteImporter_Resolve_GitExplicitWildcard(t *testing.T) {
	repoDir := initGitRepo(t, map[string]string{
		"imports/direct.yaml":      "vars:\n  direct: true\n",
		"imports/nested/deep.yaml": "vars:\n  deep: true\n",
		"imports/direct.yml.tmpl":  "vars:\n  template: true\n",
	})

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	uri := fmt.Sprintf("git::%s//imports/*.yaml?ref=main", gitFileURI(repoDir))
	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, uri+"#imports/direct.yaml", matches[0].Key)
}

func TestRemoteImporter_Resolve_GitHubShorthandSubdir(t *testing.T) {
	repoDir := initGitRepo(t, map[string]string{
		"stacks/dev.yaml": "vars:\n  shorthand: true\n",
	})

	gitConfigPath := filepath.Join(t.TempDir(), "gitconfig")
	gitConfig := fmt.Sprintf("[url %q]\n\tinsteadOf = https://github.com/acme/infrastructure\n", gitFileURI(repoDir))
	require.NoError(t, os.WriteFile(gitConfigPath, []byte(gitConfig), 0o644))
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfigPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", t.TempDir())

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	uri := "github.com/acme/infrastructure//stacks/dev?ref=main"
	matches, err := importer.Resolve(uri)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, uri+"#stacks/dev.yaml", matches[0].Key)

	data, err := os.ReadFile(matches[0].Path)
	require.NoError(t, err)
	assert.Equal(t, "vars:\n  shorthand: true\n", normalizeLineEndings(string(data)))
}

func TestRemoteImporter_ResolveNested_RemoteBase(t *testing.T) {
	repoDir := initGitRepo(t, map[string]string{
		"stacks/orgs/l360/_defaults.yaml": "import:\n  - catalog/_defaults\nvars:\n  org: l360\n",
		"stacks/catalog/_defaults.yaml":   "vars:\n  catalog: true\n",
	})

	atmosConfig := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{BasePath: "stacks"},
	}
	importer := newTestRemoteImporter(t, atmosConfig)

	uri := fmt.Sprintf("git::%s//stacks/orgs/l360/_defaults.yaml?ref=main", gitFileURI(repoDir))
	matches, err := importer.ResolveNested(uri, schema.StackImportNestedImportsRemote)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, uri+"#stacks/orgs/l360/_defaults.yaml", matches[0].Key)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(matches[0].BasePath), "/stacks"))

	data, err := os.ReadFile(filepath.Join(matches[0].BasePath, "catalog", "_defaults.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "catalog: true")
}

func TestRemoteImporter_ResolveNested_UnsupportedRawHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("vars:\n  remote: true\n"))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	_, err := importer.ResolveNested(server.URL+"/config.yaml", schema.StackImportNestedImportsRemote)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidRemoteImport)
}

func TestRemoteImporter_Download_NotFound(t *testing.T) {
	// Create a mock HTTP server that returns 404.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	// Create RemoteImporter with test cache.
	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	// Download should fail.
	_, err := importer.Download(server.URL + "/nonexistent.yaml")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDownloadRemoteImport)
}

func TestRemoteImporter_Download_LocalPath_Error(t *testing.T) {
	// Create RemoteImporter with test cache.
	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	// Trying to download a local path should fail.
	_, err := importer.Download("catalog/vpc.yaml")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidRemoteImport)
}

func TestRemoteImporter_ResolveRemoteImport_GlobalImporter(t *testing.T) {
	content := "vars:\n  global: true\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	globalImporterOnce = sync.Once{}
	globalImporter = newTestRemoteImporter(t, &schema.AtmosConfiguration{})
	globalImporterErr = nil
	t.Cleanup(func() {
		globalImporterOnce = sync.Once{}
		globalImporter = nil
		globalImporterErr = nil
	})

	uri := server.URL + "/config.yaml"
	matches, err := ResolveRemoteImport(&schema.AtmosConfiguration{}, uri)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, uri, matches[0].Key)

	data, err := os.ReadFile(matches[0].Path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestRemoteImporter_ClearCache(t *testing.T) {
	// Create a mock HTTP server.
	content := "test content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Create RemoteImporter with test cache.
	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	// Download a file.
	localPath, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)

	// Verify the file exists.
	_, err = os.Stat(localPath)
	require.NoError(t, err)

	// Clear the cache.
	err = importer.ClearCache()
	require.NoError(t, err)

	// Verify the file no longer exists.
	_, err = os.Stat(localPath)
	assert.True(t, os.IsNotExist(err))
}

func TestProcessImportPath_Local(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	basePath := filepath.Join(string(os.PathSeparator), "stacks")

	tests := []struct {
		name       string
		importPath string
		expected   string
	}{
		{"catalog path", "catalog/vpc", filepath.Join(basePath, "catalog", "vpc")},
		{"mixins path", "mixins/region", filepath.Join(basePath, "mixins", "region")},
		{"relative dot", "./local", filepath.Join(basePath, ".", "local")},
		{"relative parent", "../shared", filepath.Join(basePath, "..", "shared")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ProcessImportPath(atmosConfig, basePath, tt.importPath)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessImportPath_Remote(t *testing.T) {
	// Create a mock HTTP server.
	content := "remote: content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Reset the global importer for this test.
	globalImporterOnce = sync.Once{}
	globalImporter = nil
	globalImporterErr = nil

	atmosConfig := &schema.AtmosConfiguration{}
	basePath := filepath.Join(string(os.PathSeparator), "stacks")

	// Process a remote import path.
	result, err := ProcessImportPath(atmosConfig, basePath, server.URL+"/config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, result)
	assert.NotEqual(t, filepath.Join(basePath, server.URL+"/config.yaml"), result, "should not join remote URL with basePath")

	// Verify the downloaded file has correct content.
	data, err := os.ReadFile(result)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestResolveImportPaths_LocalPaths(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	basePath := filepath.Join(string(os.PathSeparator), "stacks")

	importPaths := []string{
		"catalog/vpc",
		"catalog/eks",
		"mixins/region/us-east-1",
	}

	resolved, err := ResolveImportPaths(atmosConfig, basePath, importPaths)
	require.NoError(t, err)
	require.Len(t, resolved, 3)

	assert.Equal(t, filepath.Join(basePath, "catalog", "vpc"), resolved[0])
	assert.Equal(t, filepath.Join(basePath, "catalog", "eks"), resolved[1])
	assert.Equal(t, filepath.Join(basePath, "mixins", "region", "us-east-1"), resolved[2])
}

func TestResolveImportPaths_EmptySlice(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	basePath := filepath.Join(string(os.PathSeparator), "stacks")

	resolved, err := ResolveImportPaths(atmosConfig, basePath, []string{})
	require.NoError(t, err)
	assert.Empty(t, resolved)
	assert.NotNil(t, resolved, "should return empty slice, not nil")
}

func TestResolveImportPaths_MixedPaths(t *testing.T) {
	// Create a mock HTTP server.
	content := "remote: content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	// Reset the global importer for this test.
	globalImporterOnce = sync.Once{}
	globalImporter = nil
	globalImporterErr = nil

	atmosConfig := &schema.AtmosConfiguration{}
	basePath := filepath.Join(string(os.PathSeparator), "stacks")

	importPaths := []string{
		"catalog/vpc",
		server.URL + "/remote.yaml",
		"catalog/eks",
	}

	resolved, err := ResolveImportPaths(atmosConfig, basePath, importPaths)
	require.NoError(t, err)
	require.Len(t, resolved, 3)

	// First and third should be local paths.
	assert.Equal(t, filepath.Join(basePath, "catalog", "vpc"), resolved[0])
	assert.Equal(t, filepath.Join(basePath, "catalog", "eks"), resolved[2])

	// Second should be a downloaded path (not the original URL).
	assert.NotEqual(t, server.URL+"/remote.yaml", resolved[1])
	assert.NotEmpty(t, resolved[1])

	// Verify the downloaded file exists and has correct content.
	data, err := os.ReadFile(resolved[1])
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestRemoteImporter_Download_MemoryCacheInvalidation(t *testing.T) {
	// Create a mock HTTP server with a request counter.
	var downloadCount atomic.Int32
	content := "remote content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadCount.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	// Download the file - should fetch from server.
	localPath, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	initialCount := downloadCount.Load()

	// Download again - should hit memory cache (no new request).
	localPath2, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	assert.Equal(t, localPath, localPath2)
	assert.Equal(t, initialCount, downloadCount.Load(), "should not re-download from memory cache")

	// Delete the cached file from disk to simulate invalidation.
	require.NoError(t, os.Remove(localPath))

	// Download again - memory cache should detect missing file and re-download.
	localPath3, err := importer.Download(server.URL + "/config.yaml")
	require.NoError(t, err)
	assert.NotEmpty(t, localPath3)
	assert.Greater(t, downloadCount.Load(), initialCount, "should re-download after file was deleted")

	// Verify the re-downloaded file has correct content.
	data, err := os.ReadFile(localPath3)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestRemoteImporter_CacheFileMemoryCacheInvalidation(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	importer := newTestRemoteImporter(t, atmosConfig)

	sourcePath := filepath.Join(t.TempDir(), "source.yaml")
	require.NoError(t, os.WriteFile(sourcePath, []byte("vars:\n  version: 1\n"), 0o644))

	cachedPath, err := importer.cacheFile("remote-key", sourcePath)
	require.NoError(t, err)
	assert.NotEmpty(t, cachedPath)

	require.NoError(t, os.WriteFile(sourcePath, []byte("vars:\n  version: 2\n"), 0o644))
	cachedPathAgain, err := importer.cacheFile("remote-key", sourcePath)
	require.NoError(t, err)
	assert.Equal(t, cachedPath, cachedPathAgain)

	data, err := os.ReadFile(cachedPathAgain)
	require.NoError(t, err)
	assert.Contains(t, string(data), "version: 1")

	require.NoError(t, os.Remove(cachedPath))

	refetchedPath, err := importer.cacheFile("remote-key", sourcePath)
	require.NoError(t, err)
	data, err = os.ReadFile(refetchedPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "version: 2")

	_, err = importer.cacheFile("missing-key", filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
}

func TestDownloadRemoteImport_GlobalImporterError(t *testing.T) {
	// Reset the global importer and inject an error.
	globalImporterOnce = sync.Once{}
	globalImporter = nil
	globalImporterErr = nil

	// Force the global importer to be initialized with an error.
	expectedErr := fmt.Errorf("cache creation failed")
	globalImporterOnce.Do(func() {
		globalImporter = nil
		globalImporterErr = expectedErr
	})

	atmosConfig := &schema.AtmosConfiguration{}
	_, err := DownloadRemoteImport(atmosConfig, "https://example.com/config.yaml")
	require.Error(t, err)
	assert.Equal(t, expectedErr, err)

	// Clean up: reset the global importer for other tests.
	globalImporterOnce = sync.Once{}
	globalImporter = nil
	globalImporterErr = nil
}

func TestResolveImportPaths_ErrorPropagation(t *testing.T) {
	// Create a mock HTTP server that returns 404.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	// Reset the global importer for this test.
	globalImporterOnce = sync.Once{}
	globalImporter = nil
	globalImporterErr = nil

	atmosConfig := &schema.AtmosConfiguration{}
	basePath := filepath.Join(string(os.PathSeparator), "stacks")

	importPaths := []string{
		"catalog/vpc",
		server.URL + "/nonexistent.yaml", // This will fail.
		"catalog/eks",
	}

	// Should return error when one path fails.
	resolved, err := ResolveImportPaths(atmosConfig, basePath, importPaths)
	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.ErrorIs(t, err, errUtils.ErrDownloadRemoteImport)
}

func TestResolveStackFiles(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"envs/dev.yaml":                 "vars:\n  env: dev\n",
		"envs/prod.yml":                 "vars:\n  env: prod\n",
		"envs/nested/region.yaml":       "vars:\n  region: use1\n",
		"envs/nested/template.yml.tmpl": "vars:\n  template: true\n",
		"envs/ignored.txt":              "ignored\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	t.Run("empty subdir", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "")
		require.Error(t, err)
		assert.Nil(t, matches)
		assert.ErrorIs(t, err, errUtils.ErrFailedToFindImport)
	})

	t.Run("no extension", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "envs/dev")
		require.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, "envs/dev.yaml", relativeSlashPath(t, root, matches[0]))
	})

	t.Run("leading slash", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "/envs/prod")
		require.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, "envs/prod.yml", relativeSlashPath(t, root, matches[0]))
	})

	t.Run("explicit file", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "envs/nested/region.yaml")
		require.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, "envs/nested/region.yaml", relativeSlashPath(t, root, matches[0]))
	})

	t.Run("directory recursive stack files only", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "envs")
		require.NoError(t, err)
		assert.Equal(t, []string{
			"envs/dev.yaml",
			"envs/nested/region.yaml",
			"envs/nested/template.yml.tmpl",
			"envs/prod.yml",
		}, relativeSlashPaths(t, root, matches))
	})

	t.Run("glob", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "envs/*.yml")
		require.NoError(t, err)
		require.Len(t, matches, 1)
		assert.Equal(t, "envs/prod.yml", relativeSlashPath(t, root, matches[0]))
	})

	t.Run("path traversal", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "../outside.yaml")
		require.Error(t, err)
		assert.Nil(t, matches)
		assert.ErrorIs(t, err, errRemoteSubdirTraversal)
	})

	t.Run("missing", func(t *testing.T) {
		matches, err := resolveStackFiles(root, "envs/missing")
		require.Error(t, err)
		assert.Nil(t, matches)
		assert.ErrorIs(t, err, errUtils.ErrFailedToFindImport)
	})
}

func TestResolveGlobPatterns_DeduplicatesAndSkipsDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"envs/dev.yaml", "envs/prod.yaml"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("vars: {}\n"), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(root, "envs", "empty"), 0o755))

	matches, err := resolveGlobPatterns(root, []string{"envs/**", "envs/dev.yaml"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"envs/dev.yaml",
		"envs/prod.yaml",
	}, relativeSlashPaths(t, root, matches))

	matches, err = resolveGlobPatterns(root, []string{"envs/*.toml"})
	require.Error(t, err)
	assert.Nil(t, matches)
	assert.ErrorIs(t, err, errUtils.ErrFailedToFindImport)
}

func TestCleanRemoteSubdir(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{name: "empty", input: "", expected: "."},
		{name: "root", input: "/", expected: "."},
		{name: "leading slash", input: "/envs/dev", expected: "envs/dev"},
		{name: "clean dot segments", input: "envs/./dev", expected: "envs/dev"},
		{name: "traversal", input: "../outside", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanRemoteSubdir(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errRemoteSubdirTraversal)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func relativeSlashPath(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	require.NoError(t, err)
	return filepath.ToSlash(rel)
}

func relativeSlashPaths(t *testing.T, root string, paths []string) []string {
	t.Helper()
	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = relativeSlashPath(t, root, path)
	}
	return result
}
