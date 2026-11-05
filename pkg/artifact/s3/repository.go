package s3

import (
	"context"
	"fmt"
	"maps"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob"
	blobs3 "github.com/cloudposse/atmos/pkg/artifact/blob/s3"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Repository is an aws/s3 artifact repository. Construct it with [New]. It is safe for concurrent
// use: all coordination happens through conditional writes and claim objects in the bucket.
type Repository struct {
	alias      string
	bucket     string
	prefix     string
	keyPattern string
	packaging  artifact.Packaging
	tags       map[string]string
	store      blob.Store
	settings   settings
}

var (
	_ artifact.Repository     = (*Repository)(nil)
	_ artifact.EvidenceReader = (*Repository)(nil)
	_ artifact.EvidenceWriter = (*Repository)(nil)
)

// New returns an aws/s3 repository for the configuration. It performs no network access and
// resolves no credentials: the AWS client, and the configured identity, are built on first use.
func New(alias string, cfg *schema.ArtifactRepositoryConfig, opts ...Option) (*Repository, error) {
	defer perf.Track(nil, "s3.New")()

	return newRepository(alias, cfg, opts, &artifact.RepositoryOptions{})
}

func newRepository(alias string, cfg *schema.ArtifactRepositoryConfig, opts []Option, ropts *artifact.RepositoryOptions) (*Repository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: repository %q has no configuration", errUtils.ErrArtifactConfigInvalid, alias)
	}
	desc, ok := artifact.BuiltinKind(artifact.RepositoryKindS3)
	if !ok {
		return nil, fmt.Errorf("%w: unknown repository kind %q", errUtils.ErrArtifactConfigInvalid, artifact.RepositoryKindS3)
	}
	if cfg.Kind != "" && cfg.Kind != artifact.RepositoryKindS3 {
		return nil, fmt.Errorf("%w: configuration kind %q cannot construct a %q repository", errUtils.ErrArtifactConfigInvalid, cfg.Kind, artifact.RepositoryKindS3)
	}
	checked := *cfg
	checked.Kind = artifact.RepositoryKindS3
	if err := artifact.ValidateRepositoryConfig(desc, &checked); err != nil {
		return nil, err
	}
	var options artifact.S3Options
	if err := artifact.DecodeOptions(cfg.Options, &options); err != nil {
		return nil, err
	}
	repo := &Repository{
		alias: alias, bucket: options.Bucket, prefix: options.Prefix, keyPattern: options.KeyPattern,
		packaging: desc.EffectivePackaging(cfg.Packaging), tags: maps.Clone(options.Tags), settings: applySettings(opts),
	}
	repo.store = repo.settings.store
	if repo.store == nil {
		store, err := buildStore(&options, cfg.Identity, ropts, &repo.settings)
		if err != nil {
			return nil, err
		}
		repo.store = store
	}
	return repo, nil
}

// buildStore constructs the AWS SDK store. It performs no I/O; credentials resolve lazily. An
// endpoint override selects the conditional-write probe, because third-party endpoints can accept
// If-None-Match and silently ignore it.
func buildStore(options *artifact.S3Options, identity string, ropts *artifact.RepositoryOptions, applied *settings) (blob.Store, error) {
	storeOpts := []blobs3.Option{
		blobs3.WithPrefix(options.Prefix),
		blobs3.WithRegion(options.Region),
		blobs3.WithTags(options.Tags),
		blobs3.WithConcurrency(applied.concurrency),
	}
	if applied.partSize > 0 {
		storeOpts = append(storeOpts, blobs3.WithPartSize(applied.partSize))
	}
	if options.Endpoint != "" {
		storeOpts = append(storeOpts, blobs3.WithEndpoint(options.Endpoint), blobs3.WithConditionalMode(blobs3.ConditionalProbe))
	}
	if options.PathStyle != nil {
		storeOpts = append(storeOpts, blobs3.WithPathStyle(*options.PathStyle))
	}
	if identity != "" {
		// A nil resolver stays a nil interface so the store fails closed instead of falling back.
		var resolver blobs3.AuthResolver
		if ropts != nil && ropts.ResolveIdentity != nil {
			resolver = identityAdapter{resolve: ropts.ResolveIdentity}
		}
		storeOpts = append(storeOpts, blobs3.WithIdentity(identity, resolver))
	}
	store, err := blobs3.New(options.Bucket, storeOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrArtifactConfigInvalid, err)
	}
	return store, nil
}

// Location returns the display locator of the repository: s3://bucket[/prefix].
func (r *Repository) Location() string {
	defer perf.Track(nil, "s3.Repository.Location")()

	if r.prefix == "" {
		return "s3://" + r.bucket
	}
	return "s3://" + r.bucket + "/" + r.prefix
}

// Capabilities implements [artifact.Repository]. It is computed from configuration alone.
func (r *Repository) Capabilities() artifact.Capabilities {
	defer perf.Track(nil, "s3.Repository.Capabilities")()

	return staticCapabilities(r.store.Capabilities().ConditionalPut, r.settings.verifier)
}

// staticCapabilities describes the kind without constructing a repository. Without conditional
// writes the repository cannot own references or claims, so it offers no lock scope and no
// conditional publication.
func staticCapabilities(conditional bool, verifier artifact.Verifier) artifact.Capabilities {
	desc, _ := artifact.BuiltinKind(artifact.RepositoryKindS3)
	caps := desc.Capabilities
	caps.Read, caps.Write = true, true
	caps.EnumerateNamed, caps.EnumerateCollection = true, true
	caps.Delete = artifact.DeleteObject
	caps.ConditionalUpdate = conditional
	caps.PreservesModes = true
	caps.SubjectKinds = []artifact.SubjectKind{artifact.SubjectRecord}
	caps.LockScopes = nil
	if conditional {
		caps.LockScopes = []artifact.LockScope{artifact.LockScopeHost, artifact.LockScopeRepository}
	}
	caps.EvidenceFormats = nil
	if verifier != nil {
		caps.EvidenceFormats = append(caps.EvidenceFormats, verifier.Formats()...)
	}
	return caps
}

// pinned builds the immutable reference for a stored Record.
func (r *Repository) pinned(ref artifact.Ref, record *artifact.Record, recordDigest digest.Digest) artifact.PinnedReference {
	return artifact.PinnedReference{
		Ref:            ref,
		Repository:     r.Location(),
		ResolvedDigest: recordDigest,
		RecordDigest:   recordDigest,
		ContentDigest:  record.ContentDigest,
	}
}

// view is a consistent, verified read of one completed artifact: its reference, Record, and
// representation.
type view struct {
	ref         *refState
	record      *artifact.Record
	recordBytes []byte
	rep         *layout.Representation
}

// unavailable builds the error for a reference that exists but cannot be used right now.
func (r *Repository) unavailable(ref *layout.Reference) error {
	return fmt.Errorf("%w: %s:%s has a pending publication by operation %s; retry the publication with the same intent to repair it",
		errUtils.ErrArtifactPublicationUnavailable, ref.Name, ref.Version, ref.OperationID)
}

// resolved is a completed reference together with the verified Record it names.
type resolved struct {
	ref         *refState
	record      *artifact.Record
	recordBytes []byte
}

// resolveRef finds the reference and Record a request names. A version reference must be
// completed. A digest reference checks the Record's embedded name and succeeds only while that
// Record is still the current completed one: retired digests are not found, never redirected to
// the current bytes.
func (r *Repository) resolveRef(ctx context.Context, ref artifact.Ref) (*resolved, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.IsDigest() {
		return r.resolveDigest(ctx, ref)
	}
	stored, err := r.loadReference(ctx, layout.ReferenceKey(ref.Name, ref.Version))
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, ref)
	}
	if !stored.ref.Available() {
		return nil, r.unavailable(stored.ref)
	}
	record, raw, err := r.loadRecord(ctx, stored.ref.RecordDigest)
	if err != nil {
		return nil, err
	}
	if err := matchesReference(stored.ref, record); err != nil {
		return nil, err
	}
	return &resolved{ref: stored, record: record, recordBytes: raw}, nil
}

func (r *Repository) resolveDigest(ctx context.Context, ref artifact.Ref) (*resolved, error) {
	record, raw, err := r.loadRecord(ctx, ref.Digest)
	if err != nil {
		return nil, err
	}
	if record.Name != ref.Name {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, ref)
	}
	stored, err := r.loadReference(ctx, layout.ReferenceKey(record.Name, record.Version))
	if err != nil {
		return nil, err
	}
	switch {
	case stored == nil:
		return nil, fmt.Errorf("%w: %s (the reference was deleted)", errUtils.ErrArtifactNotFound, ref)
	case !stored.ref.Available():
		return nil, r.unavailable(stored.ref)
	case stored.ref.RecordDigest != ref.Digest:
		return nil, fmt.Errorf("%w: %s is retired; %s:%s now resolves to another record", errUtils.ErrArtifactNotFound, ref, record.Name, record.Version)
	}
	return &resolved{ref: stored, record: record, recordBytes: raw}, nil
}

// matchesReference checks that a Record agrees with the reference that names it.
func matchesReference(ref *layout.Reference, record *artifact.Record) error {
	if record.Name != ref.Name || record.Version != ref.Version {
		return fmt.Errorf("%w: record names %s:%s but its reference is %s:%s", errUtils.ErrArtifactIntegrityFailed, record.Name, record.Version, ref.Name, ref.Version)
	}
	return nil
}

// openView loads the verified view of a pinned artifact. The pinned Record must still be the
// current completed Record of its reference.
func (r *Repository) openView(ctx context.Context, pinned *artifact.PinnedReference) (*view, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := r.resolveRef(ctx, artifact.Ref{Name: pinned.Ref.Name, Digest: pinned.RecordDigest})
	if err != nil {
		return nil, err
	}
	stored := current.ref.ref
	rep, err := r.loadRepresentation(ctx, stored.Key())
	if err != nil {
		return nil, err
	}
	if rep == nil || rep.RecordDigest != pinned.RecordDigest || rep.RepositoryKind != artifact.RepositoryKindS3 {
		return nil, fmt.Errorf("%w: %s:%s has no valid %s representation for record %s", errUtils.ErrArtifactIntegrityFailed, stored.Name, stored.Version, artifact.RepositoryKindS3, pinned.RecordDigest)
	}
	if !supportedPackaging(rep.Packaging) {
		return nil, fmt.Errorf("%w: %s:%s is stored as %q, which this repository cannot read", errUtils.ErrArtifactPackagingInvalid, stored.Name, stored.Version, rep.Packaging)
	}
	return &view{ref: current.ref, record: current.record, recordBytes: current.recordBytes, rep: rep}, nil
}

// supportedPackaging reports whether the repository can store and read the packaging.
func supportedPackaging(packaging artifact.Packaging) bool {
	switch packaging {
	case artifact.PackagingNone, artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip:
		return true
	default:
		return false
	}
}

// isArchive reports whether the packaging stores one archive object.
func isArchive(packaging artifact.Packaging) bool {
	switch packaging {
	case artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip:
		return true
	default:
		return false
	}
}
