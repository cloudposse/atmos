package artifact

import (
	"context"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// LockScope is the strength of mutation coordination a lock provides.
type LockScope string

// Lock scopes. No S3 Object Lock retention, OCI tag, or local-cache lock is a distributed lease.
const (
	// LockScopeHost serializes writers on one host. It is the default for single-host use.
	LockScopeHost LockScope = "host"

	// LockScopeRepository serializes writers across hosts through the repository's own
	// coordination (for example S3 conditional owner claims or a configured lock repository).
	LockScopeRepository LockScope = "repository"
)

// Satisfies reports whether a lock of scope s meets a request for scope required. A repository
// scope lock satisfies a host scope request; an empty requirement is always satisfied.
func (s LockScope) Satisfies(required LockScope) bool {
	defer perf.Track(nil, "artifact.LockScope.Satisfies")()

	switch required {
	case "", LockScopeHost:
		return s == LockScopeHost || s == LockScopeRepository
	case LockScopeRepository:
		return s == LockScopeRepository
	default:
		return false
	}
}

// LockRequest asks a repository to coordinate a mutation of one resource. Multi-resource work
// acquires locks in canonical repository/resource order to avoid deadlocks.
type LockRequest struct {
	// Resource identifies what is being mutated, for example a canonical reference key.
	Resource string

	// Owner identifies the writer, for example host and process.
	Owner string

	// OperationID is the logical operation identifier recorded in the claim.
	OperationID string

	// Required is the minimum guarantee the caller needs. Empty means host scope.
	Required LockScope

	// Lease is the requested lease duration for providers that expire claims. V1 claims do not
	// expire automatically, so zero means no expiry and providers without fencing must reject
	// a non-zero lease with an unsupported-capability error.
	Lease time.Duration
}

// LockHandle is an acquired, ownership-checked lock. Release verifies the ownership token and
// never releases another operation's claim. Providers never steal a lock automatically.
type LockHandle interface {
	// Resource returns the locked resource.
	Resource() string

	// Owner returns the owner recorded in the claim.
	Owner() string

	// Scope returns the actual scope provided, which results report.
	Scope() LockScope

	// Token returns the ownership token checked on renew and release.
	Token() string

	// ExpiresAt returns the lease expiry and whether the lock expires at all.
	ExpiresAt() (time.Time, bool)

	// Renew extends a leased lock; providers without leases return an unsupported-capability error.
	Renew(ctx context.Context, lease time.Duration) error

	// Release relinquishes the lock after checking the ownership token.
	Release(ctx context.Context) error
}
