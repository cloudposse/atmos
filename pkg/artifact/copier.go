package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_copier.go github.com/cloudposse/atmos/pkg/artifact Copier

// CopyFromOptions control a native transfer.
type CopyFromOptions struct {
	// ExpectedPrior is the destination reference observed under the writer guard; nil means none.
	ExpectedPrior *PinnedReference

	// Force permits replacing a conflicting destination. It never bypasses verification.
	Force bool

	// Lock is the destination lock already held by the service.
	Lock LockHandle
}

// Copier is an optional Repository extension for a compatible native transfer (for example an OCI
// registry-to-registry graph copy that preserves every original descriptor). A repository that does
// not implement it, or whose CanCopyFrom returns false, uses the shared staging fallback.
type Copier interface {
	// CanCopyFrom reports whether source is compatible with a native transfer into this repository.
	CanCopyFrom(source Repository) bool

	// CopyFrom transfers the pinned artifact from source into this repository, preserving Record
	// bytes and content identity.
	CopyFrom(ctx context.Context, source Repository, pinned PinnedReference, options CopyFromOptions) (*PublishResult, error)
}
