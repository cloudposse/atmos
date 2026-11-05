//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Inspect implements [artifact.Repository]. It reads the manifest and, for an Atmos package, the
// Record config blob; it never fetches a payload layer or a document. A foreign image, index, or
// plugin release is described from its own manifest without pretending it has an Atmos config
// blob: the inspection has no Record and carries the original root descriptor with its
// annotations and artifact type, which is what immutability and signing decisions need.
func (r *Repository) Inspect(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Inspection, error) {
	defer perf.Track(nil, "oci.Repository.Inspect")()

	st, loaded, err := r.rootOf(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	inspection := &artifact.Inspection{Pinned: pinned}
	inspection.Pinned.Repository = r.be.locator(pinned.Ref.Name)
	if loaded.class == classAtmos {
		if err := r.describeAtmos(ctx, st, loaded, inspection); err != nil {
			return nil, err
		}
	} else {
		r.describeForeign(loaded, &pinned, inspection)
	}
	// Evidence is an unverified convenience here; a listing failure must not block inspection.
	// Verification and pinning list the referrers strictly.
	if set, listErr := r.attachments(ctx, st, pinned.Ref.Name, loaded.desc); listErr == nil {
		for index := range set.valid {
			inspection.Evidence = append(inspection.Evidence, evidenceFor(&set.valid[index], loaded, r.be.locator(pinned.Ref.Name)))
		}
	}
	return inspection, nil
}

// evidenceFor describes an attachment as unverified evidence of the subject it binds.
func evidenceFor(att *attachment, loaded *root, locator string) artifact.SignatureEvidence {
	subjects := subjectsOf(loaded)
	subject := subjects[len(subjects)-1]
	for _, candidate := range subjects {
		if att.bindsTo(candidate, loaded) {
			subject = candidate
		}
	}
	return artifact.SignatureEvidence{Subject: subject, Format: artifact.EvidenceSigstoreBundle, Descriptor: att.layer, Locator: locatorDigest(locator, att.manifestDesc)}
}

// describeAtmos fills the inspection of an Atmos package.
func (r *Repository) describeAtmos(ctx context.Context, st store, loaded *root, inspection *artifact.Inspection) error {
	record, err := r.readRecord(ctx, st, inspection.Pinned.Ref.Name, loaded)
	if err != nil {
		return err
	}
	if err := requireSubjectOf(record, inspection.Pinned.Ref); err != nil {
		return err
	}
	layers, err := splitAtmosLayers(&loaded.manifest)
	if err != nil {
		return err
	}
	if err := checkDocuments(record, layers); err != nil {
		return err
	}
	inspection.Record = record
	inspection.Pinned.RecordDigest = loaded.manifest.Config.Digest
	inspection.Pinned.ContentDigest = record.ContentDigest
	inspection.Representation = artifact.Representation{
		Packaging:            layers.packaging,
		Payload:              layers.payload,
		Locator:              r.representationLocator(inspection.Pinned.Ref, loaded),
		RecordDigest:         loaded.manifest.Config.Digest,
		RepresentationDigest: layers.payload[0].Digest,
	}
	for _, role := range []artifact.DocumentRole{artifact.DocumentReadme, artifact.DocumentChangelog} {
		if doc, ok := layers.documents[role]; ok {
			inspection.Representation.Documents = append(inspection.Representation.Documents, doc)
		}
	}
	return nil
}

// checkDocuments requires the manifest's document layers to be exactly the documents the Record
// describes, by role, digest, and size.
func checkDocuments(record *artifact.Record, layers *atmosLayers) error {
	if len(record.Documents) != len(layers.documents) {
		return fmt.Errorf("%w: the Record describes %d documents but the manifest carries %d", errUtils.ErrArtifactIntegrityFailed, len(record.Documents), len(layers.documents))
	}
	for index := range record.Documents {
		doc := &record.Documents[index]
		layer, ok := layers.documents[doc.Role]
		if !ok || layer.Digest != doc.Digest || layer.Size != doc.Size {
			return fmt.Errorf("%w: the %s layer does not match the Record's descriptor", errUtils.ErrArtifactIntegrityFailed, doc.Role)
		}
	}
	return nil
}

// describeForeign fills the inspection of a foreign graph or plugin release.
func (r *Repository) describeForeign(loaded *root, pinned *artifact.PinnedReference, inspection *artifact.Inspection) {
	original := loaded.original()
	inspection.Representation = artifact.Representation{
		Packaging:    artifact.PackagingOCIGraph,
		Locator:      r.representationLocator(pinned.Ref, loaded),
		OriginalRoot: original,
	}
	switch {
	case pinned.Platform != nil && loaded.isIndex():
		for index := range loaded.index.Manifests {
			if child := loaded.index.Manifests[index]; child.Platform != nil && platformMatches(pinned.Platform, child.Platform) {
				inspection.Representation.Payload = append(inspection.Representation.Payload, child)
			}
		}
	case loaded.isIndex():
		inspection.Representation.Payload = append(inspection.Representation.Payload, loaded.index.Manifests...)
	default:
		inspection.Representation.Payload = append(inspection.Representation.Payload, loaded.manifest.Layers...)
	}
}

// representationLocator returns where the root physically lives.
func (r *Repository) representationLocator(ref artifact.Ref, loaded *root) string {
	if _, isLayout := r.be.(*layoutBackend); isLayout {
		return r.be.locator(ref.Name) + "#" + loaded.desc.Digest.String()
	}
	return locatorDigest(r.be.locator(ref.Name), loaded.desc)
}

// OpenDocument implements [artifact.Repository]. It reads the manifest and the Record, then
// streams only the selected document layer, bounded and verified against the Record. The payload
// is never downloaded. A foreign object has no documents.
func (r *Repository) OpenDocument(ctx context.Context, pinned artifact.PinnedReference, role artifact.DocumentRole) (*artifact.DocumentStream, error) {
	defer perf.Track(nil, "oci.Repository.OpenDocument")()

	st, loaded, err := r.rootOf(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	if loaded.class != classAtmos {
		return nil, fmt.Errorf("%w: %s is a %s with no Atmos package documents", errUtils.ErrArtifactDocumentMissing, pinned.ResolvedDigest, loaded.class)
	}
	record, err := r.readRecord(ctx, st, pinned.Ref.Name, loaded)
	if err != nil {
		return nil, err
	}
	if err := requireSubjectOf(record, pinned.Ref); err != nil {
		return nil, err
	}
	layers, err := splitAtmosLayers(&loaded.manifest)
	if err != nil {
		return nil, err
	}
	if err := checkDocuments(record, layers); err != nil {
		return nil, err
	}
	for index := range record.Documents {
		doc := record.Documents[index]
		if doc.Role != role {
			continue
		}
		var reader io.ReadCloser = newLazyReader(ctx, st, layers.documents[role])
		return &artifact.DocumentStream{Document: doc, Reader: reader}, nil
	}
	return nil, fmt.Errorf("%w: %s has no %s", errUtils.ErrArtifactDocumentMissing, pinned.Ref, role)
}

// Read implements [artifact.Repository]. An Atmos package yields verified payload and document
// streams; a foreign graph or plugin release yields a graph source rooted at the original
// manifest or index, so it can be transferred byte-identically. A pinned platform narrows the
// graph to that child below the unchanged original index.
func (r *Repository) Read(ctx context.Context, pinned artifact.PinnedReference) (*artifact.Content, error) {
	defer perf.Track(nil, "oci.Repository.Read")()

	st, loaded, err := r.rootOf(ctx, &pinned)
	if err != nil {
		return nil, err
	}
	inspection := &artifact.Inspection{Pinned: pinned}
	if loaded.class != classAtmos {
		r.describeForeign(loaded, &pinned, inspection)
		source := &graphSource{st: st, root: loaded.desc}
		if pinned.Platform != nil {
			child, selectErr := childFor(loaded, pinned.Platform)
			if selectErr != nil {
				return nil, selectErr
			}
			source.only = &child
		}
		return &artifact.Content{Representation: inspection.Representation, Graph: source}, nil
	}
	if err := r.describeAtmos(ctx, st, loaded, inspection); err != nil {
		return nil, err
	}
	return &artifact.Content{Record: inspection.Record, Representation: inspection.Representation, Streams: streamsOf(ctx, st, inspection)}, nil
}

// streamsOf opens the lazy, verifying streams of an Atmos package's payload and documents.
func streamsOf(ctx context.Context, st store, inspection *artifact.Inspection) []artifact.ContentStream {
	var streams []artifact.ContentStream
	for _, layer := range inspection.Representation.Payload {
		streams = append(streams, artifact.ContentStream{Descriptor: layer, Role: artifact.BlobPayload, Reader: newLazyReader(ctx, st, layer)})
	}
	for _, layer := range inspection.Representation.Documents {
		streams = append(streams, artifact.ContentStream{Descriptor: layer, Role: artifact.BlobDocument, Reader: newLazyReader(ctx, st, layer)})
	}
	return streams
}
