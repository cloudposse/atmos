package toolchain

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// downloadAsset downloads an asset to the cache directory.
func (i *Installer) downloadAsset(url string) (string, error) {
	defer perf.Track(nil, "Installer.downloadAsset")()

	// Create cache directory if it doesn't exist.
	if err := os.MkdirAll(i.cacheDir, defaultMkdirPermissions); err != nil {
		return "", fmt.Errorf("%w: failed to create cache directory: %w", ErrFileOperation, err)
	}

	// Extract filename from URL.
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]
	cachePath := filepath.Join(i.cacheDir, filename)

	// Check if already cached.
	if _, err := os.Stat(cachePath); err == nil {
		log.Debug("Using cached asset", filenameKey, filename)
		return cachePath, nil
	}

	// Download the file using authenticated HTTP client.
	log.Debug("Downloading asset", filenameKey, filename)
	return downloadToCache(url, cachePath)
}

// downloadToCache downloads a URL to the specified cache path.
func downloadToCache(url, cachePath string) (string, error) {
	defer perf.Track(nil, "downloadToCache")()

	client := httpClient.NewDefaultClient(
		httpClient.WithGitHubToken(httpClient.GetGitHubTokenFromEnv()),
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create request: %w", ErrHTTPRequest, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrDownloadFailed).
			WithExplanationf("Failed to download asset from `%s`", url).
			WithHint("Check your internet connection").
			WithHint("Verify GitHub access: `curl -I https://api.github.com`").
			WithHint("If behind proxy, ensure `HTTPS_PROXY` environment variable is set").
			WithContext("url", url).
			WithContext("error", err.Error()).
			WithExitCode(1).
			Err()
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", buildDownloadError(url, resp.StatusCode)
	}

	return writeResponseToCache(resp.Body, cachePath)
}

// writeResponseToCache reads the response body and writes it atomically to cache.
func writeResponseToCache(body io.Reader, cachePath string) (string, error) {
	defer perf.Track(nil, "writeResponseToCache")()

	var buf bytes.Buffer
	_, err := io.Copy(&buf, body)
	if err != nil {
		return "", fmt.Errorf("%w: failed to read response body: %w", ErrHTTPRequest, err)
	}

	fs := filesystem.NewOSFileSystem()
	if err := fs.WriteFileAtomic(cachePath, buf.Bytes(), defaultFileWritePermissions); err != nil {
		return "", fmt.Errorf("%w: failed to write cache file atomically: %w", ErrFileOperation, err)
	}

	return cachePath, nil
}

// buildDownloadError creates a detailed error for failed downloads.
func buildDownloadError(url string, statusCode int) error {
	defer perf.Track(nil, "buildDownloadError")()

	builder := errUtils.Build(errUtils.ErrDownloadFailed).
		WithExplanationf("Download failed with HTTP %d", statusCode).
		WithContext("url", url).
		WithContext("status_code", statusCode).
		WithExitCode(1)

	switch statusCode {
	case http.StatusNotFound:
		builder.
			WithHint("Asset not found - check tool name and version are correct").
			WithHint("Try without `v` prefix: `@1.5.0` instead of `@v1.5.0`")
	case http.StatusForbidden, http.StatusUnauthorized:
		builder.
			WithHint("GitHub API rate limit exceeded or authentication required").
			WithHint("Set `GITHUB_TOKEN` environment variable to increase rate limits").
			WithHint("Get token at: https://github.com/settings/tokens")
	default:
		builder.WithHint("Check GitHub status: https://www.githubstatus.com")
	}

	return builder.Err()
}

// downloadAssetWithVersionFallback tries the asset URL as-is, then with 'v' prefix or without, if 404.
func (i *Installer) downloadAssetWithVersionFallback(tool *registry.Tool, version, assetURL string) (string, error) {
	defer perf.Track(nil, "Installer.downloadAssetWithVersionFallback")()

	assetPath, err := i.downloadAsset(assetURL)
	if err == nil {
		return assetPath, nil
	}
	if !isHTTP404(err) {
		return "", err
	}

	return i.tryFallbackVersion(tool, version, assetURL, err)
}

// tryFallbackVersion attempts download with an alternative version prefix.
func (i *Installer) tryFallbackVersion(tool *registry.Tool, version, assetURL string, originalErr error) (string, error) {
	defer perf.Track(nil, "Installer.tryFallbackVersion")()

	var fallbackVersion string
	if strings.HasPrefix(version, versionPrefix) {
		fallbackVersion = strings.TrimPrefix(version, versionPrefix)
	} else {
		fallbackVersion = versionPrefix + version
	}

	if fallbackVersion == version {
		return "", originalErr
	}

	fallbackURL, buildErr := i.buildAssetURL(tool, fallbackVersion)
	if buildErr != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, ErrInvalidToolSpec, buildErr)
	}

	log.Debug("Asset 404, trying fallback version", "original", assetURL, "fallback", fallbackURL)
	assetPath, err := i.downloadAsset(fallbackURL)
	if err == nil {
		return assetPath, nil
	}

	return "", fmt.Errorf("%w: tried %s and %s: %w", ErrHTTPRequest, assetURL, fallbackURL, err)
}

// isHTTP404 returns true if the error is a 404 from downloadAsset.
func isHTTP404(err error) bool {
	return errors.Is(err, ErrHTTP404)
}
