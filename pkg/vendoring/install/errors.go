package install

import "errors"

// Sentinel errors for pkg/vendoring/install. These are package-local rather than centralized in
// errors/errors.go, matching the precedent pkg/vendoring/lockfile set for this refactor: they are
// internal invariants of this package's own install contract, meaningful only to this package's
// own callers and tests, not a cross-package contract other packages match against.
var (
	// ErrMixinEmpty indicates a local-file mixin was declared with an empty uri.
	ErrMixinEmpty = errors.New("mixin URI cannot be empty")
	// ErrLockDriftBlocked indicates vendor.lock.enforcement: strict rejected a pull because one or
	// more packages have drifted from their vendor.lock.yaml receipt and --refresh-lock was not
	// passed to explicitly re-resolve them.
	ErrLockDriftBlocked = errors.New("vendor lock drift blocked by enforcement: strict")
	// ErrVersionRangeRequiresGitSource indicates a semver-range `version:` was declared on a
	// source with no tag-listing mechanism in this codebase (OCI, local-file, or plain HTTP/S3).
	// Ranges are Git-only for now; see ResolveDeclaredVersion's doc comment.
	ErrVersionRangeRequiresGitSource = errors.New("a semver-range version: requires a Git source")
	// ErrCopyPackage indicates a fetched package's content failed to copy to its target path.
	ErrCopyPackage = errors.New("failed to copy package")
	// ErrRecordVendorLock indicates a vendor.yaml target's vendor.lock.yaml receipt failed to write.
	ErrRecordVendorLock = errors.New("failed to record vendor lock")
	// ErrRecordComponentVendorLock indicates a component.yaml component's vendor.lock.yaml receipt
	// failed to write.
	ErrRecordComponentVendorLock = errors.New("failed to record component vendor lock")
	// ErrRecordMixinVendorLock indicates a component.yaml mixin's vendor.lock.yaml receipt failed
	// to write.
	ErrRecordMixinVendorLock = errors.New("failed to record mixin vendor lock")
	// ErrDryRunDetectionFailed indicates a dry run's custom Git detection probe failed.
	ErrDryRunDetectionFailed = errors.New("dry-run: detection failed")
	// ErrDownloadPackage indicates a go-getter fetch of a remote source failed.
	ErrDownloadPackage = errors.New("failed to download package")
	// ErrProcessOCIImage indicates an OCI-registry source failed to pull or unpack.
	ErrProcessOCIImage = errors.New("failed to process OCI image")
)
