package source

import (
	"context"
	"fmt"
	"io"
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
	replaceTarget   bool
	baseDir         string
	preserveRuntime bool
	componentConfig map[string]any
	artifact        *downloader.ResolvedArtifact
	metadata        func(string) error
}

// WithReplaceTarget controls whether VendorSource may replace an existing target directory.
func WithReplaceTarget(replace bool) VendorSourceOption {
	return func(opts *vendorSourceOptions) {
		opts.replaceTarget = replace
	}
}

// WithBaseDir anchors a relative local-path source against baseDir instead of
// the process's current working directory. Callers that already resolve a
// working directory for the target path (e.g. the workdir step, which
// anchors `path` to `working_directory`) should pass the same directory here
// so a relative `source` resolves consistently with `path` instead of
// silently falling back to the process cwd.
func WithBaseDir(baseDir string) VendorSourceOption {
	return func(opts *vendorSourceOptions) {
		opts.baseDir = baseDir
	}
}

// VendorSource vendors a component source to the target directory.
// It uses go-getter via the existing downloader infrastructure.
// Downloads preserve caller cancellation and scoped source credentials.
func VendorSource(
	ctx context.Context,
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

	if vendorOpts.preserveRuntime {
		return vendorSourcePreserving(ctx, atmosConfig, sourceSpec, targetDir, vendorOpts)
	}
	return vendorSource(ctx, atmosConfig, sourceSpec, targetDir, vendorOpts)
}

func vendorSource(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sourceSpec *schema.VendorComponentSource, targetDir string, vendorOpts vendorSourceOptions) error {
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
	if err := validateVendorTargetDir(targetDir); err != nil {
		return err
	}

	// Resolve version into URI if specified separately.
	uri := resolveSourceURI(sourceSpec)

	// Normalize the URI for go-getter using the same logic as regular vendoring.
	uri = vendor.NormalizeURI(uri)
	if vendor.IsLocalPath(uri) && !filepath.IsAbs(uri) {
		absURI, err := resolveLocalSourcePath(uri, vendorOpts.baseDir)
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
		if err := copySourceToTarget(localDir, targetDir, sourceSpec, vendorOpts); err != nil {
			return err
		}
		return vendorOpts.captureArtifact(ctx, atmosConfig, uri, targetDir, downloader.FetchMetadata{})
	}

	return vendorRemoteSource(ctx, atmosConfig, sourceSpec, targetDir, vendorResolvedOptions{vendorSourceOptions: vendorOpts, uri: uri})
}

// downloadSourceWithMetadata populates tempDir from uri through the shared go-getter downloader
// and returns its fetch receipt. OCI sources (oci://) need no special handling: the downloader
// registers an OCI getter alongside the other protocols.
func downloadSourceWithMetadata(ctx context.Context, atmosConfig *schema.AtmosConfiguration, sourceSpec *schema.VendorComponentSource, uri, tempDir string) (downloader.FetchMetadata, error) {
	downloadOpts := []downloader.GoGetterOption{downloader.WithRetryConfig(effectiveRetryConfig(sourceSpec))}
	dl := downloader.NewGoGetterDownloader(atmosConfig, downloadOpts...)
	metadata, err := dl.(downloader.ContextFileDownloader).FetchWithMetadataContext(ctx, uri, tempDir, downloader.ClientModeAny, DefaultVendorTimeout)
	if err != nil {
		return downloader.FetchMetadata{}, errUtils.Build(errUtils.ErrSourceProvision).
			WithCause(err).
			WithExplanation("Failed to download source").
			WithContext("uri", uri).
			WithHint("Verify the source URI is accessible and credentials are configured").
			Err()
	}
	return metadata, nil
}

// Bounded default retry policy for source downloads that do not configure
// their own `retry:`. A `terraform plan` that provisions its component source
// just in time shouldn't fail on a single dropped DNS query or reset
// connection -- the same blip a hosted CI runner hits a few times a day --
// so git operations get three attempts with short exponential backoff.
// Only transient transport failures are retried (see the git getter's
// retry predicate); authentication and "not found" errors still fail fast.
const (
	defaultSourceRetryAttempts     = 3
	defaultSourceRetryInitialDelay = 1 * time.Second
	defaultSourceRetryMaxDelay     = 8 * time.Second
	defaultSourceRetryMultiplier   = 2.0
	defaultSourceRetryJitter       = 0.2
)

// defaultSourceRetryConfig returns the policy described above.
func defaultSourceRetryConfig() *schema.RetryConfig {
	attempts := defaultSourceRetryAttempts
	initialDelay := defaultSourceRetryInitialDelay
	maxDelay := defaultSourceRetryMaxDelay
	multiplier := defaultSourceRetryMultiplier
	jitter := defaultSourceRetryJitter
	return &schema.RetryConfig{
		MaxAttempts:     &attempts,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
		Multiplier:      &multiplier,
		RandomJitter:    &jitter,
		BackoffStrategy: schema.BackoffExponential,
	}
}

// effectiveRetryConfig returns the source's own `retry:` when it configured
// one, otherwise the bounded default. An explicit `max_attempts: 1` is how a
// source opts out of retrying entirely.
func effectiveRetryConfig(sourceSpec *schema.VendorComponentSource) *schema.RetryConfig {
	if sourceSpec != nil && sourceSpec.Retry != nil {
		return sourceSpec.Retry
	}
	return defaultSourceRetryConfig()
}

// localDirectorySource recognizes existing local directories while leaving remote URIs to the downloader.
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

// validateVendorTargetDir rejects empty, parent-relative, and filesystem-root provisioning destinations.
func validateVendorTargetDir(targetDir string) error {
	trimmed := strings.TrimSpace(targetDir)
	cleaned := filepath.Clean(trimmed)
	if trimmed == "" || cleaned == "." || isFilesystemRoot(cleaned) ||
		cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return errUtils.Build(errUtils.ErrUnsafeVendorTarget).
			WithContext("target", targetDir).
			WithExplanation("Refusing to provision source into an unsafe target directory").
			Err()
	}
	return nil
}

// isFilesystemRoot recognizes platform roots, including Windows drive and volume roots.
func isFilesystemRoot(path string) bool {
	if path == string(filepath.Separator) {
		return true
	}
	volume := filepath.VolumeName(path)
	if volume == "" {
		return filepath.IsAbs(path) && filepath.Dir(path) == path
	}
	return filepath.Clean(path) == filepath.Clean(volume+string(filepath.Separator))
}

// targetPathBlockedByFile reports whether the nearest existing ancestor of path is a
// regular file rather than a directory, which would prevent path (and any missing
// intermediate directories) from ever being created. It walks up from path's parent
// until it finds an ancestor that exists, or reaches the filesystem root.
func targetPathBlockedByFile(path string) bool {
	dir := filepath.Dir(path)
	for {
		info, err := os.Stat(dir)
		if err == nil {
			return !info.IsDir()
		}
		if isFilesystemRoot(dir) {
			return false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// fileURIPath converts local file URIs to native paths and leaves remote file hosts unresolved.
func fileURIPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrSourceInvalidSpec).
			WithCause(err).
			WithExplanation("Failed to parse local file URI").
			WithContext("uri", uri).
			Err()
	}
	if isWindowsDriveHost(parsed.Host) {
		return filepath.FromSlash(parsed.Host + parsed.Path), nil
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

// isWindowsDriveHost recognizes a drive letter encoded as the host of a file URI.
func isWindowsDriveHost(host string) bool {
	if len(host) != 2 || host[1] != ':' {
		return false
	}
	return (host[0] >= 'A' && host[0] <= 'Z') || (host[0] >= 'a' && host[0] <= 'z')
}

// existingDirectory returns the normalized path only when it names an existing directory.
func existingDirectory(path string) (string, bool, error) {
	cleanPath := filepath.Clean(path)
	// #nosec G703 -- local source paths are user-configured inputs that must be inspected before copying.
	info, _ := os.Stat(cleanPath)
	if info == nil || !info.IsDir() {
		return "", false, nil
	}
	return cleanPath, true, nil
}

// copySourceToTarget prepares the destination and copies files using the source include and exclude filters.
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

// singleFileInDir reports whether dir contains exactly one entry and that
// entry is a regular file rather than a directory. Go-getter's ClientModeAny
// stages a single-file source into a staging directory as one file named by
// the URL's basename (see downloader.ClientModeAny), so this is how a
// single-file `source:` is distinguished from a directory/archive source once
// staged.
func singleFileInDir(dir string) (string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to inspect downloaded source").
			WithContext("source", dir).
			Err()
	}
	if len(entries) != 1 || entries[0].IsDir() {
		return "", false, nil
	}
	return filepath.Join(dir, entries[0].Name()), true, nil
}

// copySingleFileToTarget writes a single downloaded or local file directly to
// targetDir as a file. Callers resolve the filename within the component directory.
func copySingleFileToTarget(srcFile, targetDir string, vendorOpts vendorSourceOptions) error {
	if err := os.MkdirAll(filepath.Dir(targetDir), TargetDirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to create target parent directory").
			WithContext("target", targetDir).
			Err()
	}

	info, statErr := os.Stat(targetDir)
	switch {
	case statErr == nil:
		if !vendorOpts.replaceTarget {
			return errUtils.Build(errUtils.ErrSourceCopyFailed).
				WithExplanation("Target already exists").
				WithContext("target", targetDir).
				WithHint("Remove the target or enable replacement before provisioning").
				Err()
		}
		if info.IsDir() {
			if err := os.RemoveAll(targetDir); err != nil {
				return errUtils.Build(errUtils.ErrSourceCopyFailed).
					WithCause(err).
					WithExplanation("Failed to remove existing target directory").
					WithContext("target", targetDir).
					Err()
			}
		}
	case !os.IsNotExist(statErr):
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(statErr).
			WithExplanation("Failed to inspect target directory").
			WithContext("target", targetDir).
			Err()
	}

	if err := copySingleFile(srcFile, targetDir); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).
			WithCause(err).
			WithExplanation("Failed to copy source file to target").
			WithContext("source", srcFile).
			WithContext("target", targetDir).
			Err()
	}

	return nil
}

// copySingleFile copies a single file from src to dst, preserving the source
// file's permissions.
func copySingleFile(src, dst string) error {
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

	// Close explicitly (no defer) so a delayed write failure surfaced only on
	// Close (e.g. disk full) is not silently discarded -- a deferred
	// dstFile.Close() would let VendorSource report success for an incomplete
	// target file.
	_, copyErr := io.Copy(dstFile, srcFile)
	closeErr := dstFile.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// copySingleFileToDirectory preserves the directory contract and the same filtering
// semantics as archive/directory sources while materializing an explicit file URI.
func copySingleFileToDirectory(srcFile, targetDir string, spec *schema.VendorComponentSource, opts vendorSourceOptions) error {
	if err := prepareVendorTarget(targetDir, opts); err != nil {
		return err
	}
	info, err := os.Stat(srcFile)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).WithCause(err).Err()
	}
	skip, err := vendor.CreateSkipFunc(filepath.Dir(srcFile), spec.IncludedPaths, spec.ExcludedPaths)(info, srcFile, targetDir)
	if err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).WithCause(err).Err()
	}
	if err := os.MkdirAll(targetDir, TargetDirPermissions); err != nil {
		return errUtils.Build(errUtils.ErrSourceCopyFailed).WithCause(err).Err()
	}
	if skip {
		return nil
	}
	return copySingleFileToTarget(srcFile, filepath.Join(targetDir, filepath.Base(srcFile)), opts)
}

// prepareVendorTarget creates the parent directory and enforces the configured target replacement policy.
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

// handleExistingVendorTarget removes a prior target only when replacement was explicitly enabled.
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

// resolveLocalSourcePath anchors a relative local-path uri against baseDir when
// set, otherwise against the process's current working directory (historical
// behavior, matched by filepath.Abs).
func resolveLocalSourcePath(uri, baseDir string) (string, error) {
	if baseDir != "" {
		return filepath.Join(baseDir, uri), nil
	}
	return filepath.Abs(uri)
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
