package local

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/layout"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Repository is a local directory or local archive artifact repository. Construct it with
// [NewDir] or [NewArchive]. It is safe for concurrent use: all coordination happens through
// OS file locks and atomic file replacement on disk.
type Repository struct {
	alias      string
	kind       string
	root       string
	keyPattern string
	packaging  artifact.Packaging
	store      payloadStore
	settings   settings

	// archiveMode and modeExplicit hold the quoted-octal archive permissions and whether the
	// configuration requested them explicitly. They are meaningful for archives only.
	archiveMode  fs.FileMode
	modeExplicit bool

	// hostModes reports whether the host can enforce permission bits. It is a field so tests can
	// exercise the unsupported-platform path on any host.
	hostModes bool
}

var (
	_ artifact.Repository     = (*Repository)(nil)
	_ artifact.EvidenceReader = (*Repository)(nil)
	_ artifact.EvidenceWriter = (*Repository)(nil)
)

// NewDir returns a local/dir repository for the configuration. It performs no I/O: the root is
// created by the first write.
func NewDir(alias string, cfg *schema.ArtifactRepositoryConfig, opts ...Option) (*Repository, error) {
	defer perf.Track(nil, "local.NewDir")()

	return newRepository(alias, artifact.RepositoryKindLocalDir, cfg, opts)
}

// NewArchive returns a local/archive repository for the configuration. It performs no I/O.
func NewArchive(alias string, cfg *schema.ArtifactRepositoryConfig, opts ...Option) (*Repository, error) {
	defer perf.Track(nil, "local.NewArchive")()

	return newRepository(alias, artifact.RepositoryKindLocalArchive, cfg, opts)
}

func newRepository(alias, kind string, cfg *schema.ArtifactRepositoryConfig, opts []Option) (*Repository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: repository %q has no configuration", errUtils.ErrArtifactConfigInvalid, alias)
	}
	desc, ok := artifact.BuiltinKind(kind)
	if !ok {
		return nil, fmt.Errorf("%w: unknown repository kind %q", errUtils.ErrArtifactConfigInvalid, kind)
	}
	if err := ensureKind(kind, cfg); err != nil {
		return nil, err
	}
	checked := *cfg
	checked.Kind = kind
	if err := artifact.ValidateRepositoryConfig(desc, &checked); err != nil {
		return nil, err
	}
	options := desc.NewOptions()
	if err := artifact.DecodeOptions(cfg.Options, options); err != nil {
		return nil, err
	}
	root, keyPattern, err := rootAndPattern(options)
	if err != nil {
		return nil, err
	}
	repo := &Repository{
		alias: alias, kind: kind, root: root, keyPattern: keyPattern,
		packaging: desc.EffectivePackaging(cfg.Packaging), settings: applySettings(opts),
		archiveMode: artifact.DefaultArchivePermissions, hostModes: hostEnforcesModes(),
	}
	if err := repo.configureStore(cfg.Permissions); err != nil {
		return nil, err
	}
	return repo, nil
}

// configureStore selects the payload store of the kind and, for archives, parses the permissions.
// It records whether the permissions were explicit, because an explicit request that the host
// cannot enforce is an error while a default is not.
func (r *Repository) configureStore(permissions string) error {
	if r.kind != artifact.RepositoryKindLocalArchive {
		r.store = dirStore{repo: r}
		return nil
	}
	r.store = archiveStore{repo: r}
	if permissions == "" {
		return nil
	}
	mode, err := artifact.ParsePermissions(permissions)
	if err != nil {
		return err
	}
	r.archiveMode, r.modeExplicit = mode, true
	return nil
}

// rootAndPattern extracts the absolute root and key pattern from decoded options.
func rootAndPattern(options artifact.Options) (string, string, error) {
	var path, pattern string
	switch typed := options.(type) {
	case *artifact.LocalDirOptions:
		path, pattern = typed.Path, typed.KeyPattern
	case *artifact.LocalArchiveOptions:
		path, pattern = typed.Path, typed.KeyPattern
	}
	root, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", "", fmt.Errorf("%w: repository path %q: %w", errUtils.ErrArtifactConfigInvalid, path, err)
	}
	return root, pattern, nil
}

// hostEnforcesModes reports whether the host honors Unix permission bits. Windows maps modes onto
// a read-only attribute and ACLs, so it cannot enforce an arbitrary mode.
func hostEnforcesModes() bool {
	return runtime.GOOS != "windows" && runtime.GOOS != "plan9"
}

// Root returns the absolute repository root.
func (r *Repository) Root() string {
	defer perf.Track(nil, "local.Repository.Root")()

	return r.root
}

// Capabilities implements [artifact.Repository]. It is computed from configuration alone.
func (r *Repository) Capabilities() artifact.Capabilities {
	defer perf.Track(nil, "local.Repository.Capabilities")()

	return staticCapabilities(r.kind, r.hostModes, r.settings.verifier)
}

// staticCapabilities describes a local kind without constructing a repository.
func staticCapabilities(kind string, hostModes bool, verifier artifact.Verifier) artifact.Capabilities {
	desc, _ := artifact.BuiltinKind(kind)
	caps := desc.Capabilities
	caps.Read, caps.Write = true, true
	caps.EnumerateNamed, caps.EnumerateCollection = true, true
	caps.Delete = artifact.DeleteObject
	caps.ConditionalUpdate = true
	caps.PreservesModes = hostModes || kind == artifact.RepositoryKindLocalArchive
	caps.SubjectKinds = []artifact.SubjectKind{artifact.SubjectRecord}
	caps.LockScopes = []artifact.LockScope{artifact.LockScopeHost}
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
		Repository:     r.root,
		ResolvedDigest: recordDigest,
		RecordDigest:   recordDigest,
		ContentDigest:  record.ContentDigest,
	}
}

// view is a consistent, verified read of one completed artifact: its reference, Record, and
// representation.
type view struct {
	ref         *layout.Reference
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
	ref         *layout.Reference
	record      *artifact.Record
	recordBytes []byte
}

// resolveRef finds the reference and Record a request names. A version reference must be
// completed. A digest reference checks the Record's embedded name and succeeds only while that
// Record is still the current completed one: retired digests are not found, never redirected to
// the current bytes.
func (r *Repository) resolveRef(t *tree, ref artifact.Ref) (*resolved, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.IsDigest() {
		return r.resolveDigest(t, ref)
	}
	stored, err := t.loadReference(layout.ReferenceKey(ref.Name, ref.Version))
	if err != nil {
		return nil, err
	}
	if stored == nil {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, ref)
	}
	if !stored.Available() {
		return nil, r.unavailable(stored)
	}
	record, raw, err := t.loadRecord(stored.RecordDigest)
	if err != nil {
		return nil, err
	}
	if err := matchesReference(stored, record); err != nil {
		return nil, err
	}
	return &resolved{ref: stored, record: record, recordBytes: raw}, nil
}

func (r *Repository) resolveDigest(t *tree, ref artifact.Ref) (*resolved, error) {
	record, raw, err := t.loadRecord(ref.Digest)
	if err != nil {
		return nil, err
	}
	if record.Name != ref.Name {
		return nil, fmt.Errorf("%w: %s", errUtils.ErrArtifactNotFound, ref)
	}
	stored, err := t.loadReference(layout.ReferenceKey(record.Name, record.Version))
	if err != nil {
		return nil, err
	}
	switch {
	case stored == nil:
		return nil, fmt.Errorf("%w: %s (the reference was deleted)", errUtils.ErrArtifactNotFound, ref)
	case !stored.Available():
		return nil, r.unavailable(stored)
	case stored.RecordDigest != ref.Digest:
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
func (r *Repository) openView(ctx context.Context, t *tree, pinned *artifact.PinnedReference) (*view, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := r.resolveRef(t, artifact.Ref{Name: pinned.Ref.Name, Digest: pinned.RecordDigest})
	if err != nil {
		return nil, err
	}
	stored := current.ref
	rep, err := t.loadRepresentation(stored.Key())
	if err != nil {
		return nil, err
	}
	if rep == nil || rep.RecordDigest != pinned.RecordDigest || rep.RepositoryKind != r.kind {
		return nil, fmt.Errorf("%w: %s:%s has no valid %s representation for record %s", errUtils.ErrArtifactIntegrityFailed, stored.Name, stored.Version, r.kind, pinned.RecordDigest)
	}
	if !r.store.supports(rep.Packaging) {
		return nil, fmt.Errorf("%w: %s:%s is stored as %q, which this repository cannot read", errUtils.ErrArtifactPackagingInvalid, stored.Name, stored.Version, rep.Packaging)
	}
	return &view{ref: stored, record: current.record, recordBytes: current.recordBytes, rep: rep}, nil
}
