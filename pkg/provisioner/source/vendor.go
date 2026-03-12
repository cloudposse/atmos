package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendor"
)

const (
	// DefaultVendorTimeout is the default timeout for vendor operations.
	DefaultVendorTimeout = 10 * time.Minute
	// TargetDirPermissions is the permissions for created target directories.
	TargetDirPermissions = 0o755
)

// VendorSource vendors a component source to the target directory.
// It uses go-getter via the existing downloader infrastructure.
// Note: Authentication is not yet supported - credentials must be configured
// via environment variables or cloud provider credential chains.
// Note: The context parameter is currently unused but kept for API compatibility
// with future cancellation support when the downloader is updated.
func VendorSource(
	_ context.Context, // Context kept for future cancellation support.
	atmosConfig *schema.AtmosConfiguration,
	sourceSpec *schema.VendorComponentSource,
	targetDir string,
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

	// Normalize the URI for go-getter using the same logic as regular vendoring.
	uri = vendor.NormalizeURI(uri)

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
	opts := []downloader.GoGetterOption{}
	if sourceSpec.Retry != nil {
		opts = append(opts, downloader.WithRetryConfig(sourceSpec.Retry))
	}
	dl := downloader.NewGoGetterDownloader(atmosConfig, opts...)
	err = dl.Fetch(uri, tempDir, downloader.ClientModeAny, DefaultVendorTimeout)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to download source").
			WithContext("uri", uri).
			WithHint("Verify the source URI is accessible and credentials are configured").
			Err()
	}

	// Ensure target parent directory exists.
	if err := os.MkdirAll(filepath.Dir(targetDir), TargetDirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to create target parent directory").
			WithContext("target", targetDir).
			Err()
	}

	// Remove existing target directory if it exists.
	if _, err := os.Stat(targetDir); err == nil {
		if err := os.RemoveAll(targetDir); err != nil {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithCause(err).
				WithExplanation("Failed to remove existing target directory").
				WithContext("target", targetDir).
				Err()
		}
	}

	// Copy from temp to target using the same code path as regular vendoring.
	err = vendor.CopyToTarget(tempDir, targetDir, vendor.CopyOptions{
		IncludedPaths: sourceSpec.IncludedPaths,
		ExcludedPaths: sourceSpec.ExcludedPaths,
	})
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

// copyToTarget copies downloaded source to target directory using the shared vendor copy logic.
//
// Deprecated: Use vendor.CopyToTarget directly. Kept for test compatibility.
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

	return vendor.CopyToTarget(srcDir, dstDir, vendor.CopyOptions{
		IncludedPaths: sourceSpec.IncludedPaths,
		ExcludedPaths: sourceSpec.ExcludedPaths,
	})
}
