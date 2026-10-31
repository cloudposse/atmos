package service

import (
	"context"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

//go:generate mockgen -package service -destination mock_repositories.go github.com/cloudposse/atmos/pkg/artifact/service Repositories,Journal,ImmutabilityPolicy,Repacker

// RepositoryInfo is everything the service needs to know about a configured repository without
// touching the network or resolving credentials.
type RepositoryInfo struct {
	// Alias is the configured repository alias.
	Alias string

	// Capabilities describes the repository without network access. For a repository that
	// delegates locking, it already includes the delegate's lock scopes.
	Capabilities artifact.Capabilities

	// Packaging is the effective payload packaging mode for publications.
	Packaging artifact.Packaging

	// Defaults is defaults.metadata; its labels sit under explicit publication labels.
	Defaults artifact.Metadata

	// RequiredLockScope is locking.required_scope; empty means host.
	RequiredLockScope artifact.LockScope

	// LockProvider is the alias of the repository that provides write coordination when
	// locking.repository delegates it. Empty (or equal to Alias) means the repository locks itself.
	LockProvider string

	// Policy is the effective verification policy: the global and repository trust sets, both of
	// which must be satisfied.
	Policy artifact.VerificationPolicy

	// Discovery describes where the repository's discoverable data lives, for the discovery cache
	// and catalogs. It is empty for repositories described without configuration.
	Discovery DiscoveryInfo
}

// Repositories resolves repository aliases. Describe must be free of network access and
// credential resolution so dry runs never authenticate; Open may resolve identities.
type Repositories interface {
	// Describe returns the repository description without authenticating.
	Describe(alias string) (RepositoryInfo, error)

	// Open constructs (or returns a cached) repository, resolving credentials as needed.
	Open(ctx context.Context, alias string) (artifact.Repository, error)

	// Aliases lists every configured repository alias, sorted.
	Aliases() []string
}

// JournalPhase names a step in the publication lifecycle.
type JournalPhase string

// Journal phases, in lifecycle order.
const (
	// PhasePrepared means the source snapshot and Record were sealed.
	PhasePrepared JournalPhase = "prepared"

	// PhaseLocked means the writer lock was acquired.
	PhaseLocked JournalPhase = "locked"

	// PhasePending means the publication is about to be handed to the repository.
	PhasePending JournalPhase = "pending"

	// PhaseCommitted means the repository confirmed completion.
	PhaseCommitted JournalPhase = "committed"

	// PhaseReconciled means an ambiguous failure was resolved by reading the completed reference.
	PhaseReconciled JournalPhase = "reconciled"

	// PhaseFailed means the operation failed without a committed publication.
	PhaseFailed JournalPhase = "failed"

	// PhaseReleased means the writer lock was released.
	PhaseReleased JournalPhase = "released"
)

// JournalEntry is one lifecycle event. It is an implementation recovery record, not a catalog.
type JournalEntry struct {
	// OperationID is the logical operation identifier, stable across retries.
	OperationID string

	// Phase is the lifecycle step.
	Phase JournalPhase

	// Repository is the target repository alias.
	Repository string

	// Ref is the reference being mutated.
	Ref string

	// IntentDigest is the intent digest of the artifact being published.
	IntentDigest string

	// Attempt is the one-based publication attempt.
	Attempt int

	// Err is the failure for PhaseFailed.
	Err error
}

// Journal receives publication lifecycle events. The service never blocks on or fails because of it.
type Journal interface {
	// Record observes one lifecycle event.
	Record(ctx context.Context, entry *JournalEntry)
}

// Candidate is an artifact being considered for the immutability policy.
type Candidate struct {
	// Existing reports that the candidate is the artifact already stored (not the incoming one).
	Existing bool

	// Repository is the repository alias involved.
	Repository string

	// Inspection is the metadata view, set for stored artifacts and for copy sources.
	Inspection *artifact.Inspection

	// Prepared is the artifact about to be published, when one was prepared.
	Prepared *artifact.PreparedArtifact
}

// ImmutabilityPolicy is the admission seam for immutable release formats. The OCI adapter supplies
// the plugin-format detector; the service consults it for both the stored artifact and the
// incoming one, so a generic operation can never overwrite a plugin release, with or without force.
type ImmutabilityPolicy interface {
	// Immutable reports whether the candidate is in an immutable release format and why.
	Immutable(candidate Candidate) (bool, string)
}

// ImmutabilityFunc adapts a function to [ImmutabilityPolicy].
type ImmutabilityFunc func(candidate Candidate) (bool, string)

// Immutable implements [ImmutabilityPolicy].
func (f ImmutabilityFunc) Immutable(candidate Candidate) (bool, string) {
	defer perf.Track(nil, "service.ImmutabilityFunc.Immutable")()

	return f(candidate)
}

// StaticRepositories is a fixed alias-to-repository table. Describe reads Capabilities from each
// repository (which never authenticates) and Open returns it unchanged. It suits tests and
// composition roots that construct repositories up front.
type StaticRepositories struct {
	// Repositories maps aliases to repositories.
	Repositories map[string]artifact.Repository

	// Info optionally overrides the description of an alias; the zero fields fall back to defaults.
	Info map[string]RepositoryInfo
}
