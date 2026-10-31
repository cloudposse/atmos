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

// LockNamespacer is optionally implemented by repositories to name their collection for lock keys.
// When a repository delegates locking to another repository, the delegate's lock resource is
// scoped by this namespace, so unrelated collections that share one coordinator never contend and
// two aliases for the same collection always do. A repository without it is scoped by its alias.
type LockNamespacer interface {
	// LockNamespace returns a stable, credential-free identifier of the collection, for example a
	// registry and prefix, a bucket and prefix, or an absolute directory.
	LockNamespace() string
}
