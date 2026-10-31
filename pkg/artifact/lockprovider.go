package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_lockprovider.go github.com/cloudposse/atmos/pkg/artifact LockProvider,LockHandle

// LockProvider coordinates artifact mutations. Repositories implement it directly or by composing
// a shared provider (filesystem locks, S3 conditional owner claims, a delegated lock repository).
type LockProvider interface {
	// Lock acquires an ownership-checked lock satisfying request.Required, or returns a
	// *CapabilityError wrapping ErrArtifactUnsupportedCapability when the guarantee is unavailable.
	// It never steals another operation's lock and never silently downgrades the scope.
	Lock(ctx context.Context, request LockRequest) (LockHandle, error)
}
