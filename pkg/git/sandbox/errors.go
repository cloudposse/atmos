package sandbox

import "errors"

var (
	// ErrInvalidOptions indicates an unsupported sandbox scope or option.
	ErrInvalidOptions = errors.New("invalid Git sandbox options")
	// ErrClosed indicates a sandbox that has already finished or been removed.
	ErrClosed = errors.New("git sandbox is closed or finished")
	// ErrUnsafePath indicates a path outside the checkout or inside Git metadata.
	ErrUnsafePath = errors.New("unsafe repository path")
	// ErrUnmergedIndex prevents checking an index containing unresolved conflicts.
	ErrUnmergedIndex = errors.New("resolve unmerged index entries before running repository checks")
	// ErrFixConflict means a fix could not be safely applied to the working file.
	ErrFixConflict = errors.New("formatter fix conflict")
	// ErrSupportFile indicates an invalid support-file copy.
	ErrSupportFile = errors.New("invalid sandbox support file")
	// ErrFileType indicates a file state that cannot be captured or written.
	ErrFileType = errors.New("unsupported file type")
	// ErrMissingBase identifies an unavailable comparison ref.
	ErrMissingBase = errors.New("comparison base unavailable")
)

const (
	privateFileMode = 0o600
	privateDirMode  = 0o700
	checkoutDirMode = 0o755
	gitNull         = "-z"
	gitDiff         = "diff"
	gitRevParse     = "rev-parse"
	gitNameOnly     = "--name-only"
)
