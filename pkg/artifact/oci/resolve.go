//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Resolve implements [artifact.Repository]. A tag or digest becomes a pinned handle; every
// later read uses the resolved digest, so a tag moved mid-operation is never followed. A digest
// reference selects the OCI manifest or index digest; for an Atmos package the portable Record
// digest, which is the config descriptor digest, also names the artifact.
func (r *Repository) Resolve(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "oci.Repository.Resolve")()

	_, loaded, err := r.resolveRoot(ctx, ref)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	return r.pinnedFor(ref, loaded), nil
}

// Pin implements [artifact.Repository]. It returns the immutable reference for consumer
// lockfiles: the original manifest or index digest, plus the portable Record and content digests
// bound through the config descriptor and the evidence descriptors known now. It takes no lock
// and grants no trust.
func (r *Repository) Pin(ctx context.Context, ref artifact.Ref) (artifact.PinnedReference, error) {
	defer perf.Track(nil, "oci.Repository.Pin")()

	st, loaded, err := r.resolveRoot(ctx, ref)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	pinned := r.pinnedFor(ref, loaded)
	if loaded.class == classAtmos {
		record, recordErr := r.readRecord(ctx, st, ref.Name, loaded)
		if recordErr != nil {
			return artifact.PinnedReference{}, recordErr
		}
		if recordErr = requireSubjectOf(record, ref); recordErr != nil {
			return artifact.PinnedReference{}, recordErr
		}
		pinned.ContentDigest = record.ContentDigest
	}
	attachments, err := r.attachments(ctx, st, ref.Name, loaded.desc)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	for index := range attachments.valid {
		pinned.Evidence = append(pinned.Evidence, attachments.valid[index].manifestDesc)
	}
	return pinned, nil
}

// pinnedFor builds the cheap pinned handle of a resolved root: the resolved digest, and for an
// Atmos package the Record digest read from the config descriptor without fetching the config.
func (r *Repository) pinnedFor(ref artifact.Ref, loaded *root) artifact.PinnedReference {
	pinned := artifact.PinnedReference{Ref: ref, Repository: r.be.locator(ref.Name), ResolvedDigest: loaded.desc.Digest}
	if loaded.class == classAtmos {
		pinned.RecordDigest = loaded.manifest.Config.Digest
	}
	return pinned
}

// resolveRoot resolves a reference once and classifies the root it names.
func (r *Repository) resolveRoot(ctx context.Context, ref artifact.Ref) (store, *root, error) {
	if err := ref.Validate(); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	st, err := r.be.open(ctx, ref.Name, false)
	if err != nil {
		return nil, nil, err
	}
	var desc ocispec.Descriptor
	if ref.IsDigest() {
		desc, err = r.resolveDigest(ctx, st, ref)
	} else {
		desc, err = st.Resolve(ctx, r.be.reference(ref.Name, ref.Version))
	}
	if err != nil {
		return nil, nil, resolveError(ref, err)
	}
	loaded, err := r.loadCachedRoot(ctx, st, ref.Name, desc)
	if err != nil {
		return nil, nil, err
	}
	return st, loaded, nil
}

// rootOf re-reads the root of an already pinned reference by its resolved digest.
func (r *Repository) rootOf(ctx context.Context, pinned *artifact.PinnedReference) (store, *root, error) {
	if err := artifact.ValidateName(pinned.Ref.Name); err != nil {
		return nil, nil, err
	}
	if err := artifact.ValidateDigest(pinned.ResolvedDigest); err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	st, err := r.be.open(ctx, pinned.Ref.Name, false)
	if err != nil {
		return nil, nil, err
	}
	desc, known := r.descriptorFor(pinned.Ref.Name, pinned.ResolvedDigest)
	if !known {
		desc, err = st.Resolve(ctx, pinned.ResolvedDigest.String())
		if err != nil {
			return nil, nil, resolveError(pinned.Ref, err)
		}
	}
	loaded, err := r.loadCachedRoot(ctx, st, pinned.Ref.Name, desc)
	if err != nil {
		return nil, nil, err
	}
	return st, loaded, nil
}

// resolveDigest resolves name@digest. A registry answers a direct manifest lookup in its
// repository path; a layout also requires a live reference to the digest, so a deleted artifact
// does not stay addressable. When the digest is not a manifest, it may be the Record digest of
// an Atmos package, found through the config descriptors of the name's tags.
func (r *Repository) resolveDigest(ctx context.Context, st store, ref artifact.Ref) (ocispec.Descriptor, error) {
	if _, isLayout := r.be.(*layoutBackend); !isLayout {
		desc, err := st.Resolve(ctx, ref.Digest.String())
		if err == nil || !isNotFound(err) {
			return desc, err
		}
	}
	return r.scanForDigest(ctx, st, ref)
}

// scanForDigest looks through the name's tags for a root whose manifest digest or Record digest is
// the requested digest. The scan is bounded; a name with more tags than the bound is not guessed.
func (r *Repository) scanForDigest(ctx context.Context, st store, ref artifact.Ref) (ocispec.Descriptor, error) {
	versions, err := r.be.versions(ctx, ref.Name)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	sort.Strings(versions)
	if len(versions) > maxTagsScanned {
		versions = versions[:maxTagsScanned]
	}
	for _, version := range versions {
		desc, found, err := r.digestAt(ctx, st, ref, version)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		if found {
			return desc, nil
		}
	}
	return ocispec.Descriptor{}, fmt.Errorf("%w: no root of %s has digest %s", errUtils.ErrArtifactNotFound, ref.Name, ref.Digest)
}

// digestAt reports whether the root at one version is the manifest, or the Record, that ref names.
func (r *Repository) digestAt(ctx context.Context, st store, ref artifact.Ref, version string) (ocispec.Descriptor, bool, error) {
	desc, err := st.Resolve(ctx, r.be.reference(ref.Name, version))
	switch {
	case isNotFound(err):
		return ocispec.Descriptor{}, false, nil
	case err != nil:
		return ocispec.Descriptor{}, false, err
	case desc.Digest == ref.Digest:
		return desc, true, nil
	}
	loaded, err := r.loadCachedRoot(ctx, st, ref.Name, desc)
	if err != nil {
		if ctx.Err() != nil {
			return ocispec.Descriptor{}, false, err
		}
		return ocispec.Descriptor{}, false, nil
	}
	return desc, loaded.class == classAtmos && loaded.manifest.Config.Digest == ref.Digest, nil
}

// resolveError classifies a store failure: absent content is not found, a canceled context is
// passed through, anything else is a download failure that keeps its cause.
func resolveError(ref artifact.Ref, err error) error {
	switch {
	case errors.Is(err, errUtils.ErrArtifactNotFound):
		return err
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case isNotFound(err):
		return fmt.Errorf("%w: %s: %w", errUtils.ErrArtifactNotFound, ref, err)
	default:
		return fmt.Errorf("%w: resolve %s: %w", errUtils.ErrArtifactDownloadFailed, ref, err)
	}
}

// SelectPlatform narrows a pinned multi-platform image to one platform without rewriting its
// root. The returned reference keeps the original index digest and evidence, so the original
// signature keeps covering what is verified and transferred; reading it yields a graph whose
// root is the unchanged index and whose only child is the selected platform. A reconstructed
// single-platform index would have a new digest and no signature, and this never builds one.
func (r *Repository) SelectPlatform(ctx context.Context, pinned artifact.PinnedReference, platform ocispec.Platform) (artifact.PinnedReference, ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.Repository.SelectPlatform")()

	_, loaded, err := r.rootOf(ctx, &pinned)
	if err != nil {
		return artifact.PinnedReference{}, ocispec.Descriptor{}, err
	}
	if !loaded.isIndex() {
		return artifact.PinnedReference{}, ocispec.Descriptor{}, fmt.Errorf("%w: %s is not an image index, so it has no platforms to select", errUtils.ErrArtifactRefInvalid, pinned.ResolvedDigest)
	}
	child, err := childFor(loaded, &platform)
	if err != nil {
		return artifact.PinnedReference{}, ocispec.Descriptor{}, err
	}
	selected := pinned
	selected.Platform = &platform
	return selected, child, nil
}

// childFor returns the child of an index that matches the platform.
func childFor(loaded *root, platform *ocispec.Platform) (ocispec.Descriptor, error) {
	index := slices.IndexFunc(loaded.index.Manifests, func(child ocispec.Descriptor) bool {
		return child.Platform != nil && platformMatches(platform, child.Platform)
	})
	if !loaded.isIndex() || index < 0 {
		return ocispec.Descriptor{}, fmt.Errorf("%w: platform %s/%s is not in %s", errUtils.ErrArtifactNotFound, platform.OS, platform.Architecture, loaded.desc.Digest)
	}
	return loaded.index.Manifests[index], nil
}

func platformMatches(want, have *ocispec.Platform) bool {
	return want.OS == have.OS && want.Architecture == have.Architecture && (want.Variant == "" || want.Variant == have.Variant)
}
