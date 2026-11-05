package artifact

import "context"

//go:generate mockgen -package artifact -destination mock_repository.go github.com/cloudposse/atmos/pkg/artifact Repository,GraphSource

// Repository is the storage contract implemented by every backend (local directory, local archive,
// S3, OCI layout, OCI registry). Repositories own storage mechanics and capabilities; the service
// owns name and version selection, producer preparation, metadata, digest pinning, conflict policy,
// retries, and result aggregation. Signatures, verification, pinning, and locks are repository
// contracts for every backend, never OCI-only service branches.
//
// An unavailable capability returns a *CapabilityError, never a successful no-op. Backend errors
// are wrapped in the common sentinels (errors/errors.go) while retaining their causes.
type Repository interface {
	LockProvider

	// Capabilities describes the repository without network access.
	Capabilities() Capabilities

	// Resolve resolves a reference once and returns a pinned handle used for all subsequent reads.
	Resolve(ctx context.Context, ref Ref) (PinnedReference, error)

	// Inspect returns the metadata view of a pinned artifact without downloading the payload.
	Inspect(ctx context.Context, pinned PinnedReference) (*Inspection, error)

	// OpenDocument opens one document without downloading the payload. A missing optional
	// document returns an error wrapping ErrArtifactDocumentMissing.
	OpenDocument(ctx context.Context, pinned PinnedReference, role DocumentRole) (*DocumentStream, error)

	// Read returns verified, closeable content streams or an OCI graph source for a pinned artifact.
	Read(ctx context.Context, pinned PinnedReference) (*Content, error)

	// Publish stores a prepared artifact. The expected prior reference and force policy have already
	// been checked by the service under the writer guard.
	Publish(ctx context.Context, prepared *PreparedArtifact, options PublishOptions) (*PublishResult, error)

	// List returns one page of managed references. Pagination is consumed by the service.
	List(ctx context.Context, query ListQuery) (*ListPage, error)

	// Delete removes a pinned artifact within an explicit scope after checking the expected current reference.
	Delete(ctx context.Context, pinned PinnedReference, options DeleteOptions) (*DeleteResult, error)

	// Pin resolves a reference into an immutable PinnedReference suitable for consumer lockfiles.
	// It acquires no write lock and is not a substitute for signature verification.
	Pin(ctx context.Context, ref Ref) (PinnedReference, error)

	// Sign asks signer to sign the subject and stores the returned evidence, returning its descriptors.
	Sign(ctx context.Context, subject SignatureSubject, signer Signer, options SignOptions) ([]SignatureEvidence, error)

	// Verify authenticates stored evidence against policy. It never trusts metadata claims, catalog
	// entries, labels, or cached "verified" booleans.
	Verify(ctx context.Context, subject SignatureSubject, policy VerificationPolicy) (*VerificationResult, error)
}
