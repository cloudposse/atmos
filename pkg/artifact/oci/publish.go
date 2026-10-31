//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"errors"
	"fmt"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// tagState is what a reference holds when a write begins.
type tagState struct {
	exists bool
	desc   ocispec.Descriptor
	loaded *root
}

// Publish implements [artifact.Repository]. The service has already compared intent and applied
// the force policy under the writer lock; this method still enforces the reference contract
// itself: the expected prior reference must still be current, replacement needs force, and a
// legacy plugin release is never replaced, with or without force.
//
// OCI tags have no compare-and-swap, so the prior reference is checked immediately before the
// tag moves, and the repository does not advertise conditional updates. A write whose outcome is
// unknown is reconciled by resolving the tag; there are no blind retries.
func (r *Repository) Publish(ctx context.Context, prepared *artifact.PreparedArtifact, options artifact.PublishOptions) (*artifact.PublishResult, error) {
	defer perf.Track(nil, "oci.Repository.Publish")()

	pkg, err := Assemble(prepared)
	if err != nil {
		return nil, err
	}
	ref := artifact.Ref{Name: prepared.Record.Name, Version: prepared.Record.Version}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if layout, ok := r.be.(*layoutBackend); ok {
		defer lockLayout(layout.root)()
	}
	st, err := r.be.open(ctx, ref.Name, true)
	if err != nil {
		return nil, err
	}
	reference := r.be.reference(ref.Name, ref.Version)
	state, err := r.readTag(ctx, st, ref, reference)
	if err != nil {
		return nil, err
	}
	outcome, err := decidePublish(ref, &state, &incomingRoot{digest: pkg.Manifest.Digest, class: classAtmos}, options)
	if err != nil {
		return nil, err
	}
	if outcome != artifact.OutcomeUnchanged {
		if err := r.upload(ctx, st, &uploadJob{pkg: pkg, ref: ref, reference: reference, before: state}); err != nil {
			return nil, err
		}
	}
	return r.published(ref, pkg, prepared, outcome, options), nil
}

// readTag resolves and classifies the current holder of a reference, if any.
func (r *Repository) readTag(ctx context.Context, st store, ref artifact.Ref, reference string) (tagState, error) {
	desc, err := st.Resolve(ctx, reference)
	switch {
	case err == nil:
	case isNotFound(err):
		return tagState{}, nil
	default:
		return tagState{}, fmt.Errorf("%w: resolve %s: %w", errUtils.ErrArtifactDownloadFailed, ref, err)
	}
	loaded, err := r.loadCachedRoot(ctx, st, ref.Name, desc)
	if err != nil {
		return tagState{}, err
	}
	return tagState{exists: true, desc: desc, loaded: loaded}, nil
}

// incomingRoot is what is about to be written under a reference.
type incomingRoot struct {
	digest digest.Digest
	class  rootClass
}

// decidePublish applies the reference contract to the observed state.
func decidePublish(ref artifact.Ref, state *tagState, incoming *incomingRoot, options artifact.PublishOptions) (artifact.Outcome, error) {
	if !state.exists {
		if options.ExpectedPrior != nil {
			return "", fmt.Errorf("%w: %s no longer exists but the caller expected %s", errUtils.ErrArtifactConflict, ref, options.ExpectedPrior.ResolvedDigest)
		}
		return artifact.OutcomeCreated, nil
	}
	if state.desc.Digest == incoming.digest {
		return artifact.OutcomeUnchanged, nil
	}
	if state.loaded.class == classPlugin {
		return "", fmt.Errorf("%w: %s is a legacy plugin release and stays immutable even with force", errUtils.ErrArtifactConflict, ref)
	}
	if incoming.class == classPlugin {
		return "", fmt.Errorf("%w: the incoming release for %s is a legacy plugin release and may not replace %s", errUtils.ErrArtifactConflict, ref, state.desc.Digest)
	}
	if options.ExpectedPrior == nil {
		return "", fmt.Errorf("%w: %s already exists (%s) but the caller expected it not to", errUtils.ErrArtifactConflict, ref, state.desc.Digest)
	}
	if options.ExpectedPrior.ResolvedDigest != state.desc.Digest {
		return "", fmt.Errorf("%w: %s is %s but the caller expected %s", errUtils.ErrArtifactConflict, ref, state.desc.Digest, options.ExpectedPrior.ResolvedDigest)
	}
	if !options.Force {
		return "", fmt.Errorf("%w: %s already exists with different content; use force to replace it", errUtils.ErrArtifactConflict, ref)
	}
	return artifact.OutcomeReplaced, nil
}

// uploadJob is one package upload and the tag state it started from.
type uploadJob struct {
	pkg       *Package
	ref       artifact.Ref
	reference string
	before    tagState
}

// upload writes the blobs, then the manifest under its reference.
func (r *Repository) upload(ctx context.Context, st store, job *uploadJob) error {
	pkg, ref, reference, before := job.pkg, job.ref, job.reference, job.before
	for index := range pkg.Blobs {
		if err := r.uploadBlob(ctx, st, &pkg.Blobs[index]); err != nil {
			return err
		}
	}
	if err := pushIfAbsent(ctx, st, pkg.Config, pkg.ConfigBytes); err != nil {
		return fmt.Errorf("%w: upload the Record config of %s: %w", errUtils.ErrArtifactUploadFailed, ref, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := r.readTag(ctx, st, ref, reference)
	if err != nil {
		return err
	}
	if current.exists != before.exists || current.desc.Digest != before.desc.Digest {
		return fmt.Errorf("%w: %s changed while it was being published", errUtils.ErrArtifactConflict, ref)
	}
	return r.moveTag(ctx, st, pkg, ref, reference)
}

func (r *Repository) uploadBlob(ctx context.Context, st store, blob *artifact.StagedBlob) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	exists, err := st.Exists(ctx, blob.Descriptor)
	if err != nil {
		return fmt.Errorf("%w: check %s: %w", errUtils.ErrArtifactUploadFailed, blob.Descriptor.Digest, err)
	}
	if exists {
		return nil
	}
	reader, err := blob.Open(ctx)
	if err != nil {
		return fmt.Errorf("%w: open staged blob %s: %w", errUtils.ErrArtifactUploadFailed, blob.Descriptor.Digest, err)
	}
	defer reader.Close()
	if err := st.Push(ctx, blob.Descriptor, reader); err != nil && !atmosoci.IsAlreadyExists(err) {
		return fmt.Errorf("%w: upload %s: %w", errUtils.ErrArtifactUploadFailed, blob.Descriptor.Digest, err)
	}
	return nil
}

// moveTag publishes the manifest under its reference and reconciles an unknown outcome.
func (r *Repository) moveTag(ctx context.Context, st store, pkg *Package, ref artifact.Ref, reference string) error {
	err := publishManifest(ctx, st, pkg.Manifest, pkg.ManifestBytes, reference)
	if err == nil || atmosoci.IsAlreadyExists(err) {
		return nil
	}
	if ctx.Err() != nil {
		return err
	}
	resolved, resolveErr := st.Resolve(ctx, reference)
	switch {
	case resolveErr == nil && resolved.Digest == pkg.Manifest.Digest:
		return nil
	case resolveErr == nil || isNotFound(resolveErr):
		return fmt.Errorf("%w: publish %s: %w", errUtils.ErrArtifactUploadFailed, ref, err)
	default:
		return errors.Join(errUtils.ErrArtifactPublicationAmbiguous, fmt.Errorf("publish %s failed (%w) and the tag could not be checked: %w", ref, err, resolveErr))
	}
}

// publishManifest writes a manifest under a reference, directly when the store supports it.
func publishManifest(ctx context.Context, st store, desc ocispec.Descriptor, raw []byte, reference string) error {
	if pusher, ok := st.(atmosoci.ReferencePusher); ok {
		return pusher.PushReference(ctx, desc, bytesReader(raw), reference)
	}
	if err := st.Push(ctx, desc, bytesReader(raw)); err != nil && !atmosoci.IsAlreadyExists(err) {
		return err
	}
	return st.Tag(ctx, desc, reference)
}

// published builds the result and remembers the new root so it can be signed right away.
func (r *Repository) published(ref artifact.Ref, pkg *Package, prepared *artifact.PreparedArtifact, outcome artifact.Outcome, options artifact.PublishOptions) *artifact.PublishResult {
	if loaded, err := classify(pkg.Manifest, pkg.ManifestBytes); err == nil {
		r.remember(ref.Name, pkg.Manifest, pkg.ManifestBytes)
		r.remember(ref.Name, pkg.Config, pkg.ConfigBytes)
		r.track(ref.Name, loaded)
	}
	recordDigest := pkg.Config.Digest
	representation := artifact.Representation{
		Packaging:            prepared.Packaging,
		Locator:              locatorDigest(r.be.locator(ref.Name), pkg.Manifest),
		RecordDigest:         recordDigest,
		RepresentationDigest: pkg.Blobs[0].Descriptor.Digest,
		Payload:              []ocispec.Descriptor{pkg.Blobs[0].Descriptor},
	}
	for index := 1; index < len(pkg.Blobs); index++ {
		representation.Documents = append(representation.Documents, pkg.Blobs[index].Descriptor)
	}
	result := &artifact.PublishResult{
		Outcome: outcome,
		Pinned: artifact.PinnedReference{
			Ref: ref, Repository: r.be.locator(ref.Name), ResolvedDigest: pkg.Manifest.Digest,
			RecordDigest: recordDigest, ContentDigest: prepared.Record.ContentDigest,
		},
		Representation: representation,
		Location:       r.referenceLocation(ref),
	}
	if options.Lock != nil {
		result.LockScope = options.Lock.Scope()
	}
	return result
}

// referenceLocation is the human-readable location of a name and version.
func (r *Repository) referenceLocation(ref artifact.Ref) string {
	if layout, ok := r.be.(*layoutBackend); ok {
		return layoutLocator(layout.root, ref)
	}
	return r.be.locator(ref.Name) + ":" + ref.Version
}
