package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	cp "github.com/otiai10/copy"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultVendorTimeout is the default timeout for vendor operations.
	DefaultVendorTimeout = 5 * time.Minute
	// TargetDirPermissions is the permissions for created target directories.
	TargetDirPermissions = 0o755
)

// VendorSource vendors a component source to the target directory.
// It uses go-getter via the existing downloader infrastructure.
func VendorSource(
	ctx context.Context,
	atmosConfig *schema.AtmosConfiguration,
	sourceSpec *schema.VendorComponentSource,
	targetDir string,
	authContext *schema.AuthContext,
) error {
	defer perf.Track(atmosConfig, "source.VendorSource")()

	if sourceSpec == nil {
		return errUtils.Build(errUtils.ErrNilParam).
			WithExplanation("source specification cannot be nil").
			Err()
	}

	if sourceSpec.Uri == "" {
		return errUtils.Build(errUtils.ErrSourceInvalidSpec).
			WithExplanation("source URI is required").
			Err()
	}

	// Resolve version into URI if specified separately.
	uri := resolveSourceURI(sourceSpec)

	// Normalize the URI for go-getter.
	uri = normalizeURI(uri)

	// Create temp directory for download.
	tempDir, err := os.MkdirTemp("", "atmos-source-*")
	if err != nil {
		return errUtils.Build(errUtils.ErrCreateTempDir).
			WithCause(err).
			WithExplanation("Failed to create temporary directory for download").
			Err()
	}
	defer os.RemoveAll(tempDir)

	// Download using go-getter.
	dl := downloader.NewGoGetterDownloader(atmosConfig)
	err = dl.Fetch(uri, tempDir, downloader.ClientModeDir, DefaultVendorTimeout)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to download source").
			WithContext("uri", uri).
			WithHint("Verify the source URI is accessible and credentials are configured").
			Err()
	}

	// Copy from temp to target.
	err = copyToTarget(tempDir, targetDir, sourceSpec)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to copy source to target directory").
			WithContext("source", tempDir).
			WithContext("target", targetDir).
			Err()
	}

	return nil
}

// resolveSourceURI resolves the source URI with version if specified.
func resolveSourceURI(sourceSpec *schema.VendorComponentSource) string {
	uri := sourceSpec.Uri

	// If version is specified separately, append it to the URI.
	if sourceSpec.Version != "" && !strings.Contains(uri, "?ref=") && !strings.Contains(uri, "&ref=") {
		if strings.Contains(uri, "?") {
			uri = uri + "&ref=" + sourceSpec.Version
		} else {
			uri = uri + "?ref=" + sourceSpec.Version
		}
	}

	return uri
}

// normalizeURI normalizes the URI for go-getter.
// Handles the triple-slash pattern (///) which indicates root of repository.
func normalizeURI(uri string) string {
	// Convert triple-slash to double-slash-dot for root directory.
	return strings.Replace(uri, "///", "//.", 1)
}

// copyToTarget copies downloaded source to target directory.
// Uses otiai10/copy for fast, concurrent-safe copying.
func copyToTarget(srcDir, dstDir string, sourceSpec *schema.VendorComponentSource) error {
	// Ensure target directory exists.
	if err := os.MkdirAll(filepath.Dir(dstDir), TargetDirPermissions); err != nil {
		return fmt.Errorf("failed to create target parent directory: %w", err)
	}

	// Remove existing target directory if it exists.
	if _, err := os.Stat(dstDir); err == nil {
		if err := os.RemoveAll(dstDir); err != nil {
			return fmt.Errorf("failed to remove existing target directory: %w", err)
		}
	}

	// Copy using otiai10/copy with skip patterns.
	opts := cp.Options{
		// Preserve permissions.
		PermissionControl: cp.AddPermission(0),
		// Skip symlinks in source.
		OnSymlink: func(src string) cp.SymlinkAction {
			return cp.Skip
		},
		// Skip .git directories and apply include/exclude patterns.
		Skip: createSkipFunc(srcDir, sourceSpec),
	}

	return cp.Copy(srcDir, dstDir, opts)
}

func createSkipFunc(srcDir string, sourceSpec *schema.VendorComponentSource) func(os.FileInfo, string, string) (bool, error) { //nolint:gocognit,revive,cyclop // Complex pattern matching requires nested conditions
	return func(info os.FileInfo, src, dest string) (bool, error) {
		// Always skip .git directories.
		if info.IsDir() && info.Name() == ".git" {
			return true, nil
		}

		// If no patterns specified, don't skip anything else.
		if len(sourceSpec.IncludedPaths) == 0 && len(sourceSpec.ExcludedPaths) == 0 {
			return false, nil
		}

		// Get relative path for pattern matching.
		relPath, err := filepath.Rel(srcDir, src)
		if err != nil {
			return false, err
		}

		// Check excluded paths first.
		for _, pattern := range sourceSpec.ExcludedPaths {
			matched, err := filepath.Match(pattern, relPath)
			if err != nil {
				continue
			}
			if matched {
				return true, nil
			}
			// Also check if pattern matches any parent directory.
			matched, err = filepath.Match(pattern, filepath.Base(relPath))
			if err != nil {
				continue
			}
			if matched {
				return true, nil
			}
		}

		// If included paths specified, only include matching files.
		//nolint:nestif // Nested if for pattern matching logic
		if len(sourceSpec.IncludedPaths) > 0 {
			// For directories, don't skip (we need to traverse them).
			if info.IsDir() {
				return false, nil
			}

			// For files, check if they match any included pattern.
			for _, pattern := range sourceSpec.IncludedPaths {
				matched, err := filepath.Match(pattern, relPath)
				if err != nil {
					continue
				}
				if matched {
					return false, nil
				}
				// Also check just the filename.
				matched, err = filepath.Match(pattern, filepath.Base(relPath))
				if err != nil {
					continue
				}
				if matched {
					return false, nil
				}
			}
			// File doesn't match any included pattern, skip it.
			return true, nil
		}

		return false, nil
	}
}
