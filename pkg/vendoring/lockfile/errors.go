package lockfile

import "errors"

// Sentinel errors for pkg/vendoring/lockfile. These are package-local rather than centralized in
// errors/errors.go: they're internal invariants of this package's own lock-file contract
// (corruption, security-boundary checks, I/O failures during Save/Replace/Clean), meaningful only
// to this package's own callers and tests, not a cross-package contract other packages match
// against. This mirrors the existing internal/exec/vendor*.go convention of package-local
// sentinels, applied here for the first time to this package.
var (
	// Load/Save.
	ErrReadVendorLock               = errors.New("read vendor lock")
	ErrParseVendorLock              = errors.New("parse vendor lock")
	ErrUnsupportedVendorLockVersion = errors.New("unsupported vendor lock version")
	ErrMarshalVendorLock            = errors.New("marshal vendor lock")
	ErrCreateVendorLockDir          = errors.New("create vendor lock directory")
	ErrCreateTempVendorLock         = errors.New("create temporary vendor lock")
	ErrWriteTempVendorLock          = errors.New("write temporary vendor lock")
	ErrSetVendorLockPermissions     = errors.New("set vendor lock permissions")
	ErrCloseTempVendorLock          = errors.New("close temporary vendor lock")
	ErrReplaceVendorLock            = errors.New("replace vendor lock")

	// Inventory / Record.
	ErrInventoryWalk = errors.New("inventory vendor tree")

	// Target/path security-boundary checks.
	ErrNormalizeLockTarget         = errors.New("normalize lock target")
	ErrNormalizeArtifactTarget     = errors.New("normalize artifact target")
	ErrInvalidLockOwnedFilePath    = errors.New("invalid lock-owned file path")
	ErrInvalidVendorLockTarget     = errors.New("invalid vendor lock target")
	ErrAbsoluteVendorLockTarget    = errors.New("invalid absolute vendor lock target")
	ErrVendorLockTargetEscapesRoot = errors.New("vendor lock target escapes project root")
	ErrMakeTargetRelative          = errors.New("make target relative to project root")
	ErrGetProjectRoot              = errors.New("get project root")
	ErrResolveProjectRoot          = errors.New("resolve project root")

	// Replace/Clean file operations.
	ErrInspectLockOwnedFile       = errors.New("inspect lock-owned file")
	ErrStaleLockOwnedFileModified = errors.New("stale lock-owned file was modified")
	ErrRemoveLockOwnedFile        = errors.New("remove lock-owned file")
)
