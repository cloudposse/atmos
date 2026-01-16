package installer

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestWriteResponseToCache(t *testing.T) {
	t.Run("writes content to cache file", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "cached-file.txt")
		content := []byte("test content for caching")

		reader := bytes.NewReader(content)
		resultPath, err := writeResponseToCache(reader, cachePath)

		assert.NoError(t, err)
		assert.Equal(t, cachePath, resultPath)

		// Verify file was written correctly.
		data, err := os.ReadFile(cachePath)
		assert.NoError(t, err)
		assert.Equal(t, content, data)
	})

	t.Run("writes empty content", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "empty-file.txt")
		content := []byte("")

		reader := bytes.NewReader(content)
		resultPath, err := writeResponseToCache(reader, cachePath)

		assert.NoError(t, err)
		assert.Equal(t, cachePath, resultPath)

		// Verify empty file was created.
		data, err := os.ReadFile(cachePath)
		assert.NoError(t, err)
		assert.Empty(t, data)
	})

	t.Run("writes large content", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "large-file.bin")
		// Create 1MB of test data.
		content := bytes.Repeat([]byte("x"), 1024*1024)

		reader := bytes.NewReader(content)
		resultPath, err := writeResponseToCache(reader, cachePath)

		assert.NoError(t, err)
		assert.Equal(t, cachePath, resultPath)

		// Verify large file was written correctly.
		data, err := os.ReadFile(cachePath)
		assert.NoError(t, err)
		assert.Len(t, data, len(content))
	})

	t.Run("handles read error", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "error-file.txt")

		// Create a reader that errors.
		errReader := &errorReader{err: io.ErrUnexpectedEOF}
		_, err := writeResponseToCache(errReader, cachePath)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read response body")
	})
}

func TestBuildDownloadError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{
			name:       "403 forbidden error",
			statusCode: http.StatusForbidden,
		},
		{
			name:       "401 unauthorized error",
			statusCode: http.StatusUnauthorized,
		},
		{
			name:       "500 internal server error",
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "https://github.com/owner/repo/releases/download/v1.0.0/tool.tar.gz"
			err := buildDownloadError(url, tt.statusCode)

			assert.Error(t, err)
			// The error wraps ErrDownloadFailed sentinel error.
			assert.ErrorIs(t, err, errUtils.ErrDownloadFailed)
			// Non-404 errors should NOT include ErrHTTP404.
			assert.NotErrorIs(t, err, ErrHTTP404, "Only 404 should include ErrHTTP404")
		})
	}
}

func TestBuildDownloadError_404IncludesErrHTTP404(t *testing.T) {
	// CRITICAL: This test prevents the 404 detection regression.
	// The version fallback mechanism depends on isHTTP404() detecting 404 errors.
	url := "https://example.com/asset.tar.gz"
	err := buildDownloadError(url, http.StatusNotFound)

	assert.Error(t, err)
	// Must include BOTH error sentinels for the fallback mechanism to work.
	assert.ErrorIs(t, err, ErrHTTP404, "404 errors must include ErrHTTP404 for version fallback")
	assert.ErrorIs(t, err, errUtils.ErrDownloadFailed, "404 errors must include ErrDownloadFailed")

	// Verify isHTTP404 correctly detects the error.
	assert.True(t, isHTTP404(err), "isHTTP404 must detect 404 errors from buildDownloadError")
}

func TestIsHTTP404(t *testing.T) {
	t.Run("returns true for ErrHTTP404", func(t *testing.T) {
		result := isHTTP404(ErrHTTP404)
		assert.True(t, result)
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		result := isHTTP404(ErrFileOperation)
		assert.False(t, result)
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		result := isHTTP404(nil)
		assert.False(t, result)
	})

	t.Run("returns true for wrapped ErrHTTP404", func(t *testing.T) {
		wrappedErr := wrapError(ErrHTTP404, "wrapped error")
		result := isHTTP404(wrappedErr)
		assert.True(t, result)
	})
}

func TestDownloadAsset_CacheBehavior(t *testing.T) {
	t.Run("uses cached file if exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "cache")
		require.NoError(t, os.MkdirAll(cacheDir, 0o755))

		// Create a pre-cached file.
		cachedFilename := "already-cached.tar.gz"
		cachedPath := filepath.Join(cacheDir, cachedFilename)
		require.NoError(t, os.WriteFile(cachedPath, []byte("cached content"), 0o644))

		installer := &Installer{
			cacheDir: cacheDir,
		}

		// URL ends with the cached filename - should use cache.
		url := "https://github.com/owner/repo/releases/download/v1.0.0/" + cachedFilename

		result, err := installer.downloadAsset(url)
		assert.NoError(t, err)
		assert.Equal(t, cachedPath, result)

		// Verify we got the cached content (not a download).
		content, err := os.ReadFile(result)
		assert.NoError(t, err)
		assert.Equal(t, "cached content", string(content))
	})

	t.Run("creates cache directory if it doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheDir := filepath.Join(tmpDir, "nonexistent", "deep", "cache")

		installer := &Installer{
			cacheDir: cacheDir,
		}

		// This will fail on the download (no server), but should create the dir.
		url := "https://localhost:99999/asset.tar.gz"
		_, _ = installer.downloadAsset(url)

		// Cache directory should have been created.
		info, err := os.Stat(cacheDir)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestDownloadAsset_FilenameExtraction(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		expectedFilename string
	}{
		{
			name:             "simple filename",
			url:              "https://example.com/file.tar.gz",
			expectedFilename: "file.tar.gz",
		},
		{
			name:             "deep path",
			url:              "https://github.com/owner/repo/releases/download/v1.0.0/tool-linux-amd64.tar.gz",
			expectedFilename: "tool-linux-amd64.tar.gz",
		},
		// Note: Query string test case removed - `?` is invalid in Windows filenames
		// and GitHub releases don't use query strings in asset URLs.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cacheDir := filepath.Join(tmpDir, "cache")
			require.NoError(t, os.MkdirAll(cacheDir, 0o755))

			// Pre-create the expected cached file.
			cachedPath := filepath.Join(cacheDir, tt.expectedFilename)
			require.NoError(t, os.WriteFile(cachedPath, []byte("cached"), 0o644))

			installer := &Installer{
				cacheDir: cacheDir,
			}

			result, err := installer.downloadAsset(tt.url)
			assert.NoError(t, err)
			assert.Equal(t, cachedPath, result)
		})
	}
}

// errorReader is a mock reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (n int, err error) {
	return 0, r.err
}

// wrapError wraps an error with additional context.
func wrapError(err error, msg string) error {
	return &wrappedError{err: err, msg: msg}
}

type wrappedError struct {
	err error
	msg string
}

func (e *wrappedError) Error() string {
	return e.msg + ": " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}

func TestVersionFallbackLogic(t *testing.T) {
	t.Run("adds v prefix when missing", func(t *testing.T) {
		version := "1.0.0"
		var fallbackVersion string
		if strings.HasPrefix(version, VersionPrefix) {
			fallbackVersion = strings.TrimPrefix(version, VersionPrefix)
		} else {
			fallbackVersion = VersionPrefix + version
		}
		assert.Equal(t, "v1.0.0", fallbackVersion)
	})

	t.Run("removes v prefix when present", func(t *testing.T) {
		version := "v1.0.0"
		var fallbackVersion string
		if strings.HasPrefix(version, VersionPrefix) {
			fallbackVersion = strings.TrimPrefix(version, VersionPrefix)
		} else {
			fallbackVersion = VersionPrefix + version
		}
		assert.Equal(t, "1.0.0", fallbackVersion)
	})
}
