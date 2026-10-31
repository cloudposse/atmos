//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Delete implements [artifact.Repository] with tag-safe semantics.
//
// Deleting by tag inspects every other tag of the name that resolves to the same digest. A
// registry can only delete a manifest, which removes all of its tags, so a tag delete that would
// also remove a sibling reference is rejected as an unsupported tag-only deletion. A layout
// removes just the named reference. Deleting by digest is explicitly manifest-scoped and reports
// every reference that stopped resolving. Blobs are never deleted directly, because other
// manifests may share them.
func (r *Repository) Delete(ctx context.Context, pinned artifact.PinnedReference, options artifact.DeleteOptions) (*artifact.DeleteResult, error) {
	defer perf.Track(nil, "oci.Repository.Delete")()

	name := pinned.Ref.Name
	if err := artifact.ValidateName(name); err != nil {
		return nil, err
	}
	if layout, ok := r.be.(*layoutBackend); ok {
		defer lockLayout(layout.root)()
	}
	st, loaded, err := r.rootOf(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	if options.ExpectedCurrent != nil && options.ExpectedCurrent.ResolvedDigest != loaded.desc.Digest {
		return nil, fmt.Errorf("%w: %s is %s but the caller expected %s", errUtils.ErrArtifactConflict, pinned.Ref, loaded.desc.Digest, options.ExpectedCurrent.ResolvedDigest)
	}
	if loaded.class == classPlugin {
		return nil, fmt.Errorf("%w: %s is a legacy plugin release; it is immutable and is never deleted by generic artifact operations", errUtils.ErrArtifactConflict, pinned.Ref)
	}
	holders, err := r.referencesTo(ctx, st, name, loaded.desc.Digest)
	if err != nil {
		return nil, err
	}
	if pinned.Ref.IsDigest() {
		return r.deleteDigest(ctx, loaded, name, holders)
	}
	return r.deleteTag(ctx, loaded, pinned.Ref, holders)
}

// deleteTag removes one tag, refusing when that would also remove a sibling reference.
func (r *Repository) deleteTag(ctx context.Context, loaded *root, ref artifact.Ref, holders []artifact.Ref) (*artifact.DeleteResult, error) {
	var siblings []artifact.Ref
	named := false
	for _, holder := range holders {
		if holder.Version != ref.Version {
			siblings = append(siblings, holder)
		} else {
			named = true
		}
	}
	if !named {
		return nil, fmt.Errorf("%w: %s no longer resolves to %s, so it is not deleted", errUtils.ErrArtifactConflict, ref, loaded.desc.Digest)
	}
	if r.be.deleteScope() != artifact.DeleteReference && len(siblings) > 0 {
		return nil, artifact.NewUnsupportedCapability(r.alias, "delete", "delete/tag",
			fmt.Sprintf("the registry can only delete a manifest, which would also remove %v; delete by digest to remove them together, or retag them first", siblings))
	}
	if err := r.be.remove(ctx, ref.Name, ref.Version, loaded.desc); err != nil {
		return nil, err
	}
	return &artifact.DeleteResult{Deleted: []string{r.referenceLocation(ref)}, Digest: loaded.desc.Digest}, nil
}

// deleteDigest removes the manifest and every reference to it, reporting each affected one.
func (r *Repository) deleteDigest(ctx context.Context, loaded *root, name string, holders []artifact.Ref) (*artifact.DeleteResult, error) {
	result := &artifact.DeleteResult{Digest: loaded.desc.Digest}
	if r.be.deleteScope() == artifact.DeleteReference {
		if len(holders) == 0 {
			return nil, fmt.Errorf("%w: no reference of %s resolves to %s", errUtils.ErrArtifactNotFound, name, loaded.desc.Digest)
		}
		for _, holder := range holders {
			if err := r.be.remove(ctx, holder.Name, holder.Version, loaded.desc); err != nil {
				return result, err
			}
			result.Deleted = append(result.Deleted, r.referenceLocation(holder))
			result.Affected = append(result.Affected, holder)
		}
		return result, nil
	}
	if err := r.be.remove(ctx, name, "", loaded.desc); err != nil {
		return nil, err
	}
	result.Deleted = []string{locatorDigest(r.be.locator(name), loaded.desc)}
	result.Affected = holders
	return result, nil
}

// referencesTo lists, in version order, the tags of a name that currently resolve to a digest.
// The scan is bounded; a name with more tags than the bound cannot be deleted safely and fails.
func (r *Repository) referencesTo(ctx context.Context, st store, name string, target digest.Digest) ([]artifact.Ref, error) {
	versions, err := r.be.versions(ctx, name)
	if err != nil {
		return nil, err
	}
	sort.Strings(versions)
	if len(versions) > maxTagsScanned {
		return nil, fmt.Errorf("%w: %s has %d tags; the sibling check inspects at most %d, so it cannot prove the delete is safe", errUtils.ErrArtifactDeleteFailed, name, len(versions), maxTagsScanned)
	}
	var holders []artifact.Ref
	for _, version := range versions {
		desc, err := st.Resolve(ctx, r.be.reference(name, version))
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("%w: resolve %s:%s: %w", errUtils.ErrArtifactDeleteFailed, name, version, err)
		}
		if desc.Digest == target {
			holders = append(holders, artifact.Ref{Name: name, Version: version})
		}
	}
	return holders, nil
}

// errorsIsNotFound reports whether err is the artifact not-found sentinel.
func errorsIsNotFound(err error) bool { return errors.Is(err, errUtils.ErrArtifactNotFound) }
