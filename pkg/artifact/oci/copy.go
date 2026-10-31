//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
		return artifact.Ref{Name: record.Name, Version: record.Version}, nil
	}
	if pinned.Ref.Version == "" {
		return artifact.Ref{}, fmt.Errorf("%w: copying a foreign graph needs a name:version reference to publish under", errUtils.ErrArtifactRefInvalid)
	}
	return artifact.Ref{Name: pinned.Ref.Name, Version: pinned.Ref.Version}, nil
}

// transfer copies a root and its referrers from src into this repository under ref after the
// reference contract (expected prior, force, plugin immutability) allows it.
func (r *Repository) transfer(ctx context.Context, src atmosoci.ReadOnlyGraphStore, loaded *root, ref artifact.Ref, options artifact.PublishOptions) (*artifact.PublishResult, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if layout, ok := r.be.(*layoutBackend); ok {
		defer lockLayout(layout.root)()
	}
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
	if outcome != artifact.OutcomeUnchanged {
		_, err = atmosoci.ExtendedCopy(ctx,
			atmosoci.GraphSource{Store: src, Reference: loaded.desc.Digest.String()},
			atmosoci.Destination{Store: dst, Reference: reference},
			atmosoci.WithCopyConcurrency(r.settings.concurrency))
		if err != nil {
			return nil, fmt.Errorf("%w: copy %s: %w", errUtils.ErrArtifactUploadFailed, ref, err)
		}
	}
	return r.transferred(ctx, ref, outcome, options)
}

// transferred builds the result by inspecting what the destination now holds.
func (r *Repository) transferred(ctx context.Context, ref artifact.Ref, outcome artifact.Outcome, options artifact.PublishOptions) (*artifact.PublishResult, error) {
	pinned, err := r.Pin(ctx, ref)
	if err != nil {
		return nil, err
	}
	inspection, err := r.Inspect(ctx, pinned)
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
	reference := pinned.Ref.String()
	if _, err := atmosoci.ExtendedCopy(ctx,
		atmosoci.GraphSource{Store: st, Reference: loaded.desc.Digest.String()},
		atmosoci.Destination{Store: destination, Reference: reference},
		atmosoci.WithCopyConcurrency(r.settings.concurrency)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("%w: export %s: %w", errUtils.ErrArtifactUploadFailed, pinned.Ref, err)
	}
	return loaded.desc, nil
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
		if selector == "" || entry.Digest.String() == selector || entry.Annotations[ocispec.AnnotationRefName] == selector {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 {
		return ocispec.Descriptor{}, fmt.Errorf("%w: %d roots in %q match %q; select exactly one by reference name or digest", errUtils.ErrArtifactRefInvalid, len(matches), directory, selector)
	}
	return ocispec.Descriptor{MediaType: matches[0].MediaType, Digest: matches[0].Digest, Size: matches[0].Size}, nil
}
