package source

import (
	"context"
	"fmt"
	"net/url"
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

type VendorSourceOption func(*vendorSourceOptions)

type vendorSourceOptions struct {
	replaceTarget bool
}

// WithReplaceTarget controls whether VendorSource may replace an existing target directory.
func WithReplaceTarget(replace bool) VendorSourceOption {
	return func(opts *vendorSourceOptions) {
		opts.replaceTarget = replace
	}
}

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
	options ...VendorSourceOption,
) error {
	defer perf.Track(atmosConfig, "source.VendorSource")()

	vendorOpts := vendorSourceOptions{replaceTarget: true}
	for _, option := range options {
		option(&vendorOpts)
	}

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
	if vendor.IsLocalPath(uri) && !filepath.IsAbs(uri) {
		absURI, err := filepath.Abs(uri)
		if err != nil {
			return errUtils.Build(errUtils.ErrSourceInvalidSpec).
				WithCause(err).
				WithExplanation("Failed to resolve local source path").
				WithContext("uri", uri).
				Err()
		}
		uri = absURI
	}

	if localDir, ok, err := localDirectorySource(uri); err != nil {
		return err
	} else if ok {
		return copySourceToTarget(localDir, targetDir, sourceSpec, vendorOpts)
	}

	if !vendorOpts.replaceTarget {
		if _, err := os.Stat(targetDir); err == nil {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithExplanation("Target directory already exists").
				WithContext("target", targetDir).
				WithHint("Remove the target directory or enable replacement before provisioning").
				Err()
		} else if err != nil && !os.IsNotExist(err) {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithCause(err).
				WithExplanation("Failed to inspect target directory").
				WithContext("target", targetDir).
				Err()
		}
	}

	// Generate a unique temp path for the download staging area.
	//
	// We use os.MkdirTemp to reserve a unique name, then immediately remove the
	// directory before passing the path to go-getter. This is necessary for
	// file:// URIs: go-getter's FileGetter.Get creates a symlink at dst (rather
	// than copying files), which requires dst to not already exist. If dst is
	// a pre-created real directory, FileGetter returns "destination exists and is
	// not a symlink". Removing the empty directory before the fetch lets FileGetter
	// create the symlink; the subsequent vendor.CopyToTarget then copies through it.
	// For remote sources, go-getter creates the directory itself when needed.
	tempDir, err := os.MkdirTemp("", "atmos-source-*")
	if err != nil {
		return errUtils.Build(errUtils.ErrCreateTempDir).
			WithCause(err).
			WithExplanation("Failed to create temporary directory for download").
			Err()
	}
	// Remove the empty directory so go-getter can create it as a symlink (for
	// file:// URIs) or recreate it as a real directory (for remote URIs).
	if removeErr := os.Remove(tempDir); removeErr != nil && !os.IsNotExist(removeErr) {
		return errUtils.Build(errUtils.ErrCreateTempDir).
			WithCause(removeErr).
			WithExplanation("Failed to prepare temporary directory for download").
			Err()
	}
	defer os.RemoveAll(tempDir)

	// Download using go-getter.
	downloadOpts := []downloader.GoGetterOption{}
	if sourceSpec.Retry != nil {
		downloadOpts = append(downloadOpts, downloader.WithRetryConfig(sourceSpec.Retry))
	}
	dl := downloader.NewGoGetterDownloader(atmosConfig, downloadOpts...)
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

	// Remove existing target directory if it exists and replacement is allowed.
	if _, err := os.Stat(targetDir); err == nil {
		if !vendorOpts.replaceTarget {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithExplanation("Target directory already exists").
				WithContext("target", targetDir).
				WithHint("Remove the target directory or enable replacement before provisioning").
				Err()
		}
		if err := os.RemoveAll(targetDir); err != nil {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithCause(err).
				WithExplanation("Failed to remove existing target directory").
				WithContext("target", targetDir).
				Err()
		}
	} else if err != nil && !os.IsNotExist(err) {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to inspect target directory").
			WithContext("target", targetDir).
			Err()
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

func localDirectorySource(uri string) (string, bool, error) {
	switch {
	case vendor.IsFileURI(uri):
		path, err := fileURIPath(uri)
		if err != nil || path == "" {
			return "", false, err
		}
		return existingDirectory(path)
	case vendor.IsLocalPath(uri):
		return existingDirectory(uri)
	default:
		return "", false, nil
	}
}

func fileURIPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrSourceInvalidSpec).
			WithCause(err).
			WithExplanation("Failed to parse local file URI").
			WithContext("uri", uri).
			Err()
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return "", nil
	}
	path := parsed.Path
	if path == "" {
		path = parsed.Opaque
	}
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path), nil
}

func existingDirectory(path string) (string, bool, error) {
	cleanPath := filepath.Clean(path)
	// #nosec G703 -- local source paths are user-configured inputs that must be inspected before copying.
	info, _ := os.Stat(cleanPath)
	if info == nil || !info.IsDir() {
		return "", false, nil
	}
	return cleanPath, true, nil
}

func copySourceToTarget(
	sourceDir string,
	targetDir string,
	sourceSpec *schema.VendorComponentSource,
	vendorOpts vendorSourceOptions,
) error {
	if err := prepareVendorTarget(targetDir, vendorOpts); err != nil {
		return err
	}

	if err := vendor.CopyToTarget(sourceDir, targetDir, vendor.CopyOptions{
		IncludedPaths: sourceSpec.IncludedPaths,
		ExcludedPaths: sourceSpec.ExcludedPaths,
	}); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to copy source to target directory").
			WithContext("source", sourceDir).
			WithContext("target", targetDir).
			Err()
	}

	return nil
}

func prepareVendorTarget(targetDir string, vendorOpts vendorSourceOptions) error {
	if err := os.MkdirAll(filepath.Dir(targetDir), TargetDirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to create target parent directory").
			WithContext("target", targetDir).
			Err()
	}

	if _, err := os.Stat(targetDir); err == nil {
		return handleExistingVendorTarget(targetDir, vendorOpts)
	} else if !os.IsNotExist(err) {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to inspect target directory").
			WithContext("target", targetDir).
			Err()
	}

	return nil
}

func handleExistingVendorTarget(targetDir string, vendorOpts vendorSourceOptions) error {
	if !vendorOpts.replaceTarget {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithExplanation("Target directory already exists").
			WithContext("target", targetDir).
			WithHint("Remove the target directory or enable replacement before provisioning").
			Err()
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to remove existing target directory").
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
