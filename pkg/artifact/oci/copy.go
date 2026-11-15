//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CanCopyFrom implements [artifact.Copier]. Another OCI repository from this package can always
// transfer a graph natively: every original manifest, blob, and referrer is copied byte for byte,
// so the resolved digest and the signatures survive.
func (r *Repository) CanCopyFrom(source artifact.Repository) bool {
	defer perf.Track(nil, "oci.Repository.CanCopyFrom")()

	other, ok := source.(*Repository)
	return ok && other != r
}

// CopyFrom implements [artifact.Copier]. It copies the pinned root with its whole graph and every
// referrer (signatures, SBOMs) to this repository through the policy-aware transport. Nothing is
// repackaged: an Atmos package and a foreign image alike keep their original raw manifests, so
// the destination resolves to the source digest. A registry that cannot list or copy the
// required referrers fails the copy; signature material is never silently omitted.
func (r *Repository) CopyFrom(ctx context.Context, source artifact.Repository, pinned artifact.PinnedReference, options artifact.CopyFromOptions) (*artifact.PublishResult, error) {
	defer perf.Track(nil, "oci.Repository.CopyFrom")()

	other, ok := source.(*Repository)
	if !ok {
		return nil, artifact.NewUnsupportedCapability(r.alias, "copy", "transfer/native", "only OCI repositories transfer graphs natively")
	}
	if pinned.Platform != nil {
		return nil, artifact.NewUnsupportedCapability(r.alias, "copy", "transfer/platform", "a native copy transfers the whole image index; narrowing to one platform is available through Read's graph source")
	}
	st, loaded, err := other.rootOf(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	ref, err := other.destinationRef(ctx, st, loaded, &pinned)
	if err != nil {
		return nil, err
	}
	return r.transfer(ctx, st, loaded, ref, artifact.PublishOptions(options))
}

// destinationRef names the reference a graph is published under: the Record's name and version for
// an Atmos package, or the requested name:version for a foreign graph.
func (r *Repository) destinationRef(ctx context.Context, st store, loaded *root, pinned *artifact.PinnedReference) (artifact.Ref, error) {
	if loaded.class == classAtmos {
		record, err := r.readRecord(ctx, st, pinned.Ref.Name, loaded)
		if err != nil {
			return artifact.Ref{}, err
		}
		// The source was resolved without reading its Record, so the Record is bound to the
		// reference it was resolved by before it chooses the destination. Otherwise a tag that
		// serves another artifact's package would pick that artifact's reference here, and the
		// destination check would compare the Record with itself.
		if err := requireSubjectOf(record, pinned.Ref); err != nil {
			return artifact.Ref{}, err
		}
		return artifact.Ref{Name: record.Name, Version: record.Version}, nil
	}
	if pinned.Ref.Version == "" {
		return artifact.Ref{}, fmt.Errorf("%w: copying a foreign graph needs a name:version reference to publish under", errUtils.ErrArtifactRefInvalid)
	}
	return artifact.Ref{Name: pinned.Ref.Name, Version: pinned.Ref.Version}, nil
}

// transfer copies a root and its referrers from src into this repository under ref after the
// reference contract (expected prior, force, plugin immutability) allows it.
func (r *Repository) transfer(ctx context.Context, src atmosoci.ReadOnlyGraphStore, loaded *root, ref artifact.Ref, options artifact.PublishOptions) (result *artifact.PublishResult, err error) {
	if err := requireDestinationVersion(ref); err != nil {
		return nil, err
	}
	// A package is checked as a whole before any lock is taken or the destination is touched: the
	// tag moves once the graph has been copied, so a malformed package must never get that far.
	if err := r.requireImportable(ctx, src, loaded, ref); err != nil {
		return nil, err
	}
	release, err := r.guardLayout(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	releaseReference, err := r.guardReference(ctx, ref, options.Lock)
	if err != nil {
		return nil, err
	}
	defer func() {
		if releaseErr := releaseReference(); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
	}()
	dst, err := r.be.open(ctx, ref.Name, true)
	if err != nil {
		return nil, err
	}
	reference := r.be.reference(ref.Name, ref.Version)
	state, err := r.readTag(ctx, dst, ref, reference)
	if err != nil {
		return nil, err
	}
	outcome, err := decidePublish(ref, &state, &incomingRoot{digest: loaded.desc.Digest, class: loaded.class}, options)
	if err != nil {
		return nil, err
	}
	{
		_, err = atmosoci.ExtendedCopy(ctx,
			atmosoci.GraphSource{Store: src, Reference: loaded.desc.Digest.String()},
			atmosoci.Destination{Store: dst, Reference: reference},
			atmosoci.WithCopyConcurrency(r.settings.concurrency))
		if err != nil {
			return nil, fmt.Errorf("%w: copy %s: %w", errUtils.ErrArtifactUploadFailed, ref, err)
		}
	}
	pinned, err := r.pinCopied(ctx, dst, loaded, ref)
	if err != nil {
		return nil, err
	}
	return r.transferred(ctx, &pinned, ref, outcome, options)
}

// guardReference keeps the registry copy's read-decide-write sequence under one lock. A registry
// tag has no compare-and-swap, and the tag is written unconditionally once the graph has been
// transferred, so the policy decision made from the tag read is only as good as the exclusion that
// protects the tag until the write. A caller that holds the reference lock already has that
// exclusion; a caller that passes none gets the reference lock taken here, before the tag is
// read. A layout is protected by its write guard instead, so it takes no reference lock.
func (r *Repository) guardReference(ctx context.Context, ref artifact.Ref, held artifact.LockHandle) (func() error, error) {
	if r.be.kind() != artifact.RepositoryKindOCIRegistry || held != nil {
		return func() error { return nil }, nil
	}
	handle, err := r.Lock(ctx, artifact.LockRequest{Resource: ref.String()})
	if err != nil {
		return nil, fmt.Errorf("lock %s before copying into repository %q: %w", ref, r.alias, err)
	}
	return func() error { return handle.Release(ctx) }, nil
}

// pinCopied pins the digest that was copied, read back from the destination. The destination tag
// is deliberately not resolved again: another writer can move it after the copy, and the pin must
// describe the content this operation published.
func (r *Repository) pinCopied(ctx context.Context, dst store, copied *root, ref artifact.Ref) (artifact.PinnedReference, error) {
	landed, err := r.loadCachedRoot(ctx, dst, ref.Name, copied.desc)
	if err != nil {
		return artifact.PinnedReference{}, err
	}
	return r.pinRoot(ctx, dst, ref, landed)
}

// transferred builds the result by inspecting the pinned content the destination now holds.
func (r *Repository) transferred(ctx context.Context, pinned *artifact.PinnedReference, ref artifact.Ref, outcome artifact.Outcome, options artifact.PublishOptions) (*artifact.PublishResult, error) {
	inspection, err := r.Inspect(ctx, *pinned)
	if err != nil {
		return nil, err
	}
	result := &artifact.PublishResult{
		Outcome: outcome, Pinned: inspection.Pinned, Representation: inspection.Representation, Location: r.referenceLocation(ref),
	}
	if options.Lock != nil {
		result.LockScope = options.Lock.Scope()
	}
	return result, nil
}

// ImportOptions control importing a graph from an OCI layout payload.
type ImportOptions struct {
	// Select picks the root inside the payload by its reference name or digest. Empty requires
	// the payload to hold exactly one root.
	Select string

	// ExpectedPrior is the reference the caller observed at the destination; nil means none.
	ExpectedPrior *artifact.PinnedReference

	// Force permits replacing a conflicting reference. It never replaces a plugin release.
	Force bool

	// Lock is the writer lock already held by the caller.
	Lock artifact.LockHandle
}

// ImportLayout imports an OCI content graph from a standard OCI image layout directory (the
// carrier used for plugin releases, offline media, and graph payloads) under ref. The layout is
// opened read-only. The original manifests, blobs, and referrers are copied unchanged, so the
// imported root keeps its digest and signatures; a foreign graph is never repackaged.
func (r *Repository) ImportLayout(ctx context.Context, directory string, ref artifact.Ref, options ImportOptions) (*artifact.PublishResult, error) {
	defer perf.Track(nil, "oci.Repository.ImportLayout")()

	if err := requireDestinationVersion(ref); err != nil {
		return nil, err
	}
	desc, err := layoutRoot(directory, options.Select)
	if err != nil {
		return nil, err
	}
	src, err := atmosoci.OpenLayoutReadOnly(ctx, directory)
	if err != nil {
		return nil, fmt.Errorf("%w: open layout %q: %w", errUtils.ErrArtifactDownloadFailed, directory, err)
	}
	loaded, err := loadRoot(ctx, src, desc)
	if err != nil {
		return nil, err
	}
	return r.transfer(ctx, src, loaded, ref, artifact.PublishOptions{ExpectedPrior: options.ExpectedPrior, Force: options.Force, Lock: options.Lock})
}

// requireDestinationVersion rejects a destination without a version. A reference may name a digest
// to resolve with, but a copy or import publishes a name:version tag: a digest-only destination
// would be stored under an empty version, which artifact listing never exposes.
func requireDestinationVersion(ref artifact.Ref) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if ref.Version == "" {
		return fmt.Errorf("%w: %s needs a version to be published under, not only a digest", errUtils.ErrArtifactRefInvalid, ref)
	}
	return nil
}

// requireImportable rejects a package that every later read would reject, before the transfer can
// publish it. The Record binds an Atmos package to its own name and version, the manifest must
// carry exactly the archive payload and the documents the Record describes, and every descriptor
// must be well formed. A foreign graph carries no Record and is published under the requested
// reference as it is.
func (r *Repository) requireImportable(ctx context.Context, src atmosoci.Fetcher, loaded *root, ref artifact.Ref) error {
	if loaded.class != classAtmos {
		return nil
	}
	record, err := r.readRecord(ctx, src, ref.Name, loaded)
	if err != nil {
		return err
	}
	if err := requireSubjectOf(record, ref); err != nil {
		return err
	}
	layers, err := splitAtmosLayers(&loaded.manifest)
	if err != nil {
		return err
	}
	return checkDocuments(record, layers)
}

// ExportLayout exports the pinned graph, with every referrer, to a standard OCI image layout
// directory holding exactly one root, so it can travel as a payload and be imported again with its
// digest intact. It returns the exported root descriptor.
func (r *Repository) ExportLayout(ctx context.Context, pinned artifact.PinnedReference, directory string) (ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.Repository.ExportLayout")()

	st, loaded, err := r.rootOf(ctx, &pinned)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	destination, err := atmosoci.OpenLayout(ctx, directory)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: open layout %q: %w", errUtils.ErrArtifactUploadFailed, directory, err)
	}
	if err := validateExportRoots(ctx, destination, directory, loaded.desc.Digest); err != nil {
		return ocispec.Descriptor{}, err
	}
	reference := pinned.Ref.String()
	if _, err := atmosoci.ExtendedCopy(ctx,
		atmosoci.GraphSource{Store: st, Reference: loaded.desc.Digest.String()},
		atmosoci.Destination{Store: destination, Reference: reference},
		atmosoci.WithCopyConcurrency(r.settings.concurrency)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: export %s: %w", errUtils.ErrArtifactUploadFailed, pinned.Ref, err)
	}
	return loaded.desc, nil
}

// validateExportRoots permits the requested root and unnamed referrers, but rejects other roots.
func validateExportRoots(ctx context.Context, destination store, directory string, target digest.Digest) error {
	raw, err := os.ReadFile(filepath.Join(directory, ocispec.ImageIndexFile))
	if err != nil {
		return fmt.Errorf("%w: read export index: %w", errUtils.ErrArtifactUploadFailed, err)
	}
	var index ocispec.Index
	if err := json.Unmarshal(raw, &index); err != nil {
		return fmt.Errorf("%w: parse export index: %w", errUtils.ErrArtifactUploadFailed, err)
	}
	for _, entry := range index.Manifests {
		if entry.Digest == target {
			continue
		}
		if entry.Annotations[ocispec.AnnotationRefName] == "" {
			raw, err := fetchBounded(ctx, destination, entry, maxManifestSize)
			if err != nil {
				return err
			}
			var manifest ocispec.Manifest
			if json.Unmarshal(raw, &manifest) == nil && manifest.Subject != nil {
				continue
			}
		}
		return fmt.Errorf("%w: export layout already contains another root %s", errUtils.ErrArtifactConflict, entry.Digest)
	}
	return nil
}

// layoutRoot reads a layout's index.json and picks the root to import.
func layoutRoot(directory, selector string) (ocispec.Descriptor, error) {
	raw, err := os.ReadFile(filepath.Join(directory, ocispec.ImageIndexFile))
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: read the layout index of %q: %w", errUtils.ErrArtifactNotFound, directory, err)
	}
	var index ocispec.Index
	if err := json.Unmarshal(raw, &index); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: layout index of %q: %w", errUtils.ErrArtifactIntegrityFailed, directory, err)
	}
	var matches []ocispec.Descriptor
	for _, entry := range index.Manifests {
		if matchesLayoutSelector(entry, selector) && !slices.ContainsFunc(matches, func(seen ocispec.Descriptor) bool { return seen.Digest == entry.Digest }) {
			// Two reference names for one root digest select one root, not two.
			matches = append(matches, entry)
		}
	}
	if selector == "" && len(matches) == 0 && len(index.Manifests) == 1 {
		matches = index.Manifests
	}
	if len(matches) != 1 {
		return ocispec.Descriptor{}, fmt.Errorf("%w: %d roots in %q match %q; select exactly one by reference name or digest", errUtils.ErrArtifactRefInvalid, len(matches), directory, selector)
	}
	return ocispec.Descriptor{MediaType: matches[0].MediaType, Digest: matches[0].Digest, Size: matches[0].Size}, nil
}
