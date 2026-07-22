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
)
