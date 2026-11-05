package catalog

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// BuildRequest resolves inventory references into a catalog snapshot. Build never writes to a
// repository and never publishes.
type BuildRequest struct {
	// Repository is the configured repository alias the references are relative to.
	Repository string

	// Refs are the explicit references to resolve, relative to Repository.
	Refs []artifact.Ref

	// Name is the catalog manifest name; empty uses the configured catalog name, or "catalog".
	Name string
}

// BuildResult is the snapshot plus the catalog state it was based on.
type BuildResult struct {
	// File is the catalog snapshot. Its revision is the revision of the latest published catalog it
	// was based on (zero when none exists or none is configured), so a later publish detects that the
	// catalog changed in between.
	File *File

	// BaseDigest is the OCI manifest digest of the latest published catalog the snapshot was based on;
	// empty when there was none. Pass it to publish as the expected digest.
	BaseDigest string
}

// Build resolves each reference and verifies its Record metadata, reading no payload, and returns a
// catalog snapshot. A repository without a configured catalog still builds; only the base revision
// is then zero.
func (s *Service) Build(ctx context.Context, request *BuildRequest) (*BuildResult, error) {
	defer perf.Track(nil, "catalog.Service.Build")()

	t, err := s.describe(request.Repository, false)
	if err != nil {
		return nil, err
	}
	repo, err := s.repos.Open(ctx, request.Repository)
	if err != nil {
		return nil, err
	}
	file := New(nameOf(t, request.Name))
	result := &BuildResult{File: file}
	if t.uri != "" {
		base, err := latest(ctx, repo, t.ref)
		if err != nil {
			return nil, err
		}
		if base != nil {
			file.Metadata.Revision, result.BaseDigest = base.File.Metadata.Revision, base.Digest.String()
		}
	}
	listed, err := s.resolve(ctx, repo, t, request.Refs)
	if err != nil {
		return nil, err
	}
	for index := range listed {
		if _, err := Upsert(file, &listed[index].entry, &listed[index].version); err != nil {
			return nil, err
		}
	}
	Normalize(file)
	return result, nil
}

// PublishRequest publishes a built snapshot to the repository's configured catalog.
type PublishRequest struct {
	// Repository is the configured repository alias.
	Repository string

	// File is the snapshot to publish. It replaces the catalog's entries.
	File *File

	// ExpectedDigest, when set, must equal the digest of the catalog currently published.
	ExpectedDigest string
}

// Result describes a catalog mutation.
type Result struct {
	// Repository is the configured repository alias.
	Repository string `json:"repository"`

	// Catalog is the configured catalog reference.
	Catalog string `json:"catalog"`

	// Digest is the OCI manifest digest of the catalog after the write.
	Digest string `json:"digest,omitempty"`

	// PreviousDigest is the digest of the catalog before the write; empty when it did not exist.
	PreviousDigest string `json:"previous_digest,omitempty"`

	// Revision is the catalog revision after the write.
	Revision int `json:"revision"`

	// Listed is the number of name:version pairs the catalog lists after the write.
	Listed int `json:"listed"`

	// Added are the references newly listed.
	Added []string `json:"added,omitempty"`

	// Updated are the references whose listing now points at a different digest.
	Updated []string `json:"updated,omitempty"`

	// Removed are the references unlisted.
	Removed []string `json:"removed,omitempty"`

	// LockScope is the actual scope of the lock held during the write; empty when none was available.
	LockScope artifact.LockScope `json:"lock_scope,omitempty"`
}

// Publish replaces the configured catalog with a built snapshot. It fails before any write when the
// repository has no catalog configured, when the snapshot names locations outside the repository,
// or when the catalog changed since the snapshot was built (revision or digest).
func (s *Service) Publish(ctx context.Context, request *PublishRequest) (*Result, error) {
	defer perf.Track(nil, "catalog.Service.Publish")()

	t, err := s.describeForWrite(request.Repository)
	if err != nil {
		return nil, err
	}
	if err := Validate(request.File, t.info.Discovery.Location); err != nil {
		return nil, err
	}
	return s.mutate(ctx, t, func(_ context.Context, _ artifact.Repository, current *Loaded) (*File, *change, error) {
		if err := checkFresh(request.File, request.ExpectedDigest, current); err != nil {
			return nil, nil, err
		}
		next := *request.File
		next.Spec.Entries = append([]Entry(nil), request.File.Spec.Entries...)
		return &next, &change{added: diff(current, &next)}, nil
	})
}

// checkFresh rejects a snapshot built on a catalog that has since changed.
func checkFresh(file *File, expectedDigest string, current *Loaded) error {
	if current == nil {
		if file.Metadata.Revision != 0 || expectedDigest != "" {
			return fmt.Errorf("%w: the catalog does not exist (the snapshot was based on revision %d); rebuild it", errUtils.ErrArtifactCatalogConflict, file.Metadata.Revision)
		}
		return nil
	}
	if current.File.Metadata.Revision != file.Metadata.Revision {
		return fmt.Errorf("%w: the snapshot is based on revision %d but the catalog is at revision %d; rebuild it from the latest catalog", errUtils.ErrArtifactCatalogConflict, file.Metadata.Revision, current.File.Metadata.Revision)
	}
	if expectedDigest != "" && expectedDigest != current.Digest.String() {
		return fmt.Errorf("%w: expected catalog digest %s but the catalog is %s", errUtils.ErrArtifactCatalogConflict, expectedDigest, current.Digest)
	}
	return nil
}

// AddRequest lists explicit releases in the configured catalog.
type AddRequest struct {
	// Repository is the configured repository alias.
	Repository string

	// Refs are the releases to list, relative to Repository.
	Refs []artifact.Ref
}

// Add resolves releases and lists them in the latest catalog, creating the catalog when none exists.
func (s *Service) Add(ctx context.Context, request *AddRequest) (*Result, error) {
	defer perf.Track(nil, "catalog.Service.Add")()

	t, err := s.describeForWrite(request.Repository)
	if err != nil {
		return nil, err
	}
	return s.mutate(ctx, t, func(ctx context.Context, repo artifact.Repository, current *Loaded) (*File, *change, error) {
		return s.merge(ctx, repo, t, current, request.Refs)
	})
}

// SyncRequest merges an inventory's releases into the configured catalog.
type SyncRequest struct {
	// Repository is the configured repository alias.
	Repository string

	// Refs are the inventory references, relative to Repository.
	Refs []artifact.Ref
}

// Sync merges the verified releases of an inventory into the latest catalog and keeps every entry
// the inventory does not mention.
func (s *Service) Sync(ctx context.Context, request *SyncRequest) (*Result, error) {
	defer perf.Track(nil, "catalog.Service.Sync")()

	t, err := s.describeForWrite(request.Repository)
	if err != nil {
		return nil, err
	}
	return s.mutate(ctx, t, func(ctx context.Context, repo artifact.Repository, current *Loaded) (*File, *change, error) {
		return s.merge(ctx, repo, t, current, request.Refs)
	})
}

// RemoveRequest unlists one version from the configured catalog.
type RemoveRequest struct {
	// Repository is the configured repository alias.
	Repository string

	// Ref is the listed name and version.
	Ref artifact.Ref
}

// Remove unlists a version. It never deletes the artifact, its signatures, or a consumer lock file.
func (s *Service) Remove(ctx context.Context, request *RemoveRequest) (*Result, error) {
	defer perf.Track(nil, "catalog.Service.Remove")()

	if request.Ref.Version == "" {
		return nil, fmt.Errorf("%w: unlisting needs a name:version reference, not a digest", errUtils.ErrArtifactRefInvalid)
	}
	t, err := s.describeForWrite(request.Repository)
	if err != nil {
		return nil, err
	}
	return s.mutate(ctx, t, func(_ context.Context, _ artifact.Repository, current *Loaded) (*File, *change, error) {
		if current == nil {
			return nil, nil, fmt.Errorf("%w: repository %q has no published catalog to unlist from", errUtils.ErrArtifactNotFound, request.Repository)
		}
		next := clone(current.File)
		if err := Unlist(next, request.Ref.Name, request.Ref.Version); err != nil {
			return nil, nil, err
		}
		return next, &change{removed: []string{request.Ref.String()}}, nil
	})
}

// describeForWrite resolves the configuration of a mutating command, before any I/O.
func (s *Service) describeForWrite(alias string) (*target, error) {
	t, err := s.describe(alias, true)
	if err != nil {
		return nil, err
	}
	return t, t.requireWritable()
}

// change records what a mutation did to the listing.
type change struct {
	added, updated, removed []string
}

// mutator computes the next catalog from the latest one read inside the write guard.
type mutator func(ctx context.Context, repo artifact.Repository, current *Loaded) (*File, *change, error)

// mutate runs a read-latest, modify, publish cycle inside the repository's write guard. The latest
// catalog is read after the guard is taken, so a serialized writer never builds on a stale copy.
func (s *Service) mutate(ctx context.Context, t *target, apply mutator) (*Result, error) {
	repo, err := s.repos.Open(ctx, t.alias)
	if err != nil {
		return nil, err
	}
	operationID := s.newID()
	guard, err := s.acquire(ctx, t, repo, operationID)
	if err != nil {
		return nil, err
	}
	result, err := s.write(ctx, &session{target: t, repo: repo, guard: guard, operationID: operationID}, apply)
	return result, errors.Join(err, guard.release(ctx))
}

// session is one guarded read-latest, modify, publish cycle.
type session struct {
	target      *target
	repo        artifact.Repository
	guard       *guard
	operationID string
}

func (s *Service) write(ctx context.Context, run *session, apply mutator) (*Result, error) {
	t, repo, guard, operationID := run.target, run.repo, run.guard, run.operationID
	current, err := latest(ctx, repo, t.ref)
	if err != nil {
		return nil, err
	}
	next, changed, err := apply(ctx, repo, current)
	if err != nil {
		return nil, err
	}
	next.Metadata.Revision = revisionOf(current) + 1
	next.Metadata.GeneratedAt = s.clock().UTC().Format("2006-01-02T15:04:05Z")
	Normalize(next)
	if err := Validate(next, t.info.Discovery.Location); err != nil {
		return nil, err
	}
	options := &WriteOptions{Lock: guard.handle(), Now: s.clock(), OperationID: operationID}
	result := &Result{Repository: t.alias, Catalog: t.uri, Revision: next.Metadata.Revision, Listed: count(next), LockScope: guard.scope()}
	if current != nil {
		options.Prior, result.PreviousDigest = &current.Pinned, current.Digest.String()
	}
	published, err := Write(ctx, repo, t.ref, next, options)
	if err != nil {
		return nil, err
	}
	result.Digest = published.Pinned.ResolvedDigest.String()
	result.Added, result.Updated, result.Removed = changed.added, changed.updated, changed.removed
	return result, nil
}

func revisionOf(current *Loaded) int {
	if current == nil {
		return 0
	}
	return current.File.Metadata.Revision
}

// listing is one resolved release.
type listing struct {
	entry   Entry
	version Version
}

// resolve reads the Record metadata of references, bounded and in order. It never downloads a
// payload: it resolves each reference and inspects it.
func (s *Service) resolve(ctx context.Context, repo artifact.Repository, t *target, refs []artifact.Ref) ([]listing, error) {
	listed := make([]listing, len(refs))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(s.concurrency)
	for index := range refs {
		group.Go(func() error {
			pinned, err := repo.Resolve(groupCtx, refs[index])
			if err != nil {
				return fmt.Errorf("resolve %s in repository %q: %w", refs[index], t.alias, err)
			}
			inspection, err := repo.Inspect(groupCtx, pinned)
			if err != nil {
				return fmt.Errorf("inspect %s in repository %q: %w", refs[index], t.alias, err)
			}
			entry, version, err := FromInspection(t.info.Discovery.Location, inspection)
			if err != nil {
				return err
			}
			listed[index] = listing{entry: entry, version: version}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return listed, nil
}

// merge lists resolved releases in the latest catalog and keeps everything else.
func (s *Service) merge(ctx context.Context, repo artifact.Repository, t *target, current *Loaded, refs []artifact.Ref) (*File, *change, error) {
	file := New(nameOf(t, ""))
	if current != nil {
		file = clone(current.File)
	}
	listed, err := s.resolve(ctx, repo, t, refs)
	if err != nil {
		return nil, nil, err
	}
	changed := &change{}
	for index := range listed {
		known := hasVersion(file, listed[index].entry.Name, listed[index].version.Version)
		moved, err := Upsert(file, &listed[index].entry, &listed[index].version)
		if err != nil {
			return nil, nil, err
		}
		label := artifact.Ref{Name: listed[index].entry.Name, Version: listed[index].version.Version}.String()
		switch {
		case moved:
			changed.updated = append(changed.updated, label)
		case !known:
			changed.added = append(changed.added, label)
		}
	}
	return file, changed, nil
}

func hasVersion(file *File, name, version string) bool {
	for index := range file.Spec.Entries {
		if file.Spec.Entries[index].Name != name {
			continue
		}
		for position := range file.Spec.Entries[index].Versions {
			if file.Spec.Entries[index].Versions[position].Version == version {
				return true
			}
		}
	}
	return false
}

// diff lists the references a replacement snapshot adds relative to the current catalog.
func diff(current *Loaded, next *File) []string {
	var added []string
	for index := range next.Spec.Entries {
		entry := &next.Spec.Entries[index]
		for position := range entry.Versions {
			if current == nil || !hasVersion(current.File, entry.Name, entry.Versions[position].Version) {
				added = append(added, artifact.Ref{Name: entry.Name, Version: entry.Versions[position].Version}.String())
			}
		}
	}
	return added
}

func count(file *File) int {
	total := 0
	for index := range file.Spec.Entries {
		total += len(file.Spec.Entries[index].Versions)
	}
	return total
}

// clone deep-copies a catalog so a mutation never aliases the loaded copy.
func clone(file *File) *File {
	out := *file
	out.Spec.Entries = make([]Entry, len(file.Spec.Entries))
	for index := range file.Spec.Entries {
		entry := file.Spec.Entries[index]
		entry.Versions = make([]Version, len(file.Spec.Entries[index].Versions))
		for position := range file.Spec.Entries[index].Versions {
			version := file.Spec.Entries[index].Versions[position]
			version.Labels = copyLabels(version.Labels)
			version.Documents = append([]Document(nil), version.Documents...)
			entry.Versions[position] = version
		}
		out.Spec.Entries[index] = entry
	}
	return &out
}

// nameOf picks the catalog manifest name.
func nameOf(t *target, requested string) string {
	switch {
	case requested != "":
		return requested
	case t.ref.Name != "":
		return t.ref.Name
	default:
		return "catalog"
	}
}
