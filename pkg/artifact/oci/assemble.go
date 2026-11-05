//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"encoding/json"
	"fmt"
	"path"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ociSchemaVersion is the only OCI manifest schema version.
const ociSchemaVersion = 2

// Package is an assembled Atmos package ready to upload: the config blob (the canonical Record
// bytes), the deterministic manifest, and the staged blobs in manifest layer order.
type Package struct {
	// Config describes the config blob. Its digest is the Record digest.
	Config ocispec.Descriptor

	// ConfigBytes are the canonical Record bytes.
	ConfigBytes []byte

	// Manifest describes the manifest.
	Manifest ocispec.Descriptor

	// ManifestBytes are the exact manifest bytes whose digest is Manifest.Digest.
	ManifestBytes []byte

	// Blobs are the payload and document blobs in manifest layer order.
	Blobs []artifact.StagedBlob
}

// Assemble builds the manifest of a new Atmos package from a prepared artifact.
//
// The config blob is the canonical Record, byte for byte what local and S3 records store. The
// layers are the single archive payload followed by the README and CHANGELOG documents as
// separate Markdown blobs with explicit roles and titles. The creation time is the Record's
// captured time, so assembly reads no clock and identical input yields an identical manifest
// digest.
func Assemble(prepared *artifact.PreparedArtifact) (*Package, error) {
	defer perf.Track(nil, "oci.Assemble")()

	if prepared == nil {
		return nil, fmt.Errorf("%w: no prepared artifact", errUtils.ErrArtifactPackagingInvalid)
	}
	record := &prepared.Record
	configBytes, err := record.Canonical()
	if err != nil {
		return nil, err
	}
	payload, documents, err := stagedLayers(prepared)
	if err != nil {
		return nil, err
	}
	pkg := &Package{
		Config:      ocispec.Descriptor{MediaType: artifact.MediaTypeConfig, Digest: digest.FromBytes(configBytes), Size: int64(len(configBytes))},
		ConfigBytes: configBytes,
	}
	layers := []ocispec.Descriptor{payload.layer}
	pkg.Blobs = append(pkg.Blobs, payload.blob)
	for _, doc := range documents {
		layers = append(layers, doc.layer)
		pkg.Blobs = append(pkg.Blobs, doc.blob)
	}
	manifest := ocispec.Manifest{
		Versioned:    specVersion(),
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: artifactTypeFor(record.Kind),
		Config:       pkg.Config,
		Layers:       layers,
		Annotations:  manifestAnnotations(record),
	}
	pkg.ManifestBytes, err = json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: manifest: %w", errUtils.ErrArtifactPackagingInvalid, err)
	}
	pkg.Manifest = ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromBytes(pkg.ManifestBytes), Size: int64(len(pkg.ManifestBytes))}
	return pkg, nil
}

// stagedBlob pairs a manifest layer with the staged blob that uploads it.
type stagedBlob struct {
	layer ocispec.Descriptor
	blob  artifact.StagedBlob
}

// stagedLayers selects the one archive payload and the document blobs the Record describes,
// checking each against the Record and the declared packaging.
func stagedLayers(prepared *artifact.PreparedArtifact) (stagedBlob, []stagedBlob, error) {
	mediaType, ok := mediaTypeOf(prepared.Packaging)
	if !ok {
		return stagedBlob{}, nil, fmt.Errorf("%w: OCI repositories store tar, tgz, or zip archives, not %q", errUtils.ErrArtifactPackagingInvalid, prepared.Packaging)
	}
	if prepared.Packaging == artifact.PackagingCatalog && prepared.Record.Kind != artifact.KindCatalog {
		return stagedBlob{}, nil, fmt.Errorf("%w: catalog packaging is reserved for Records of kind %q, not %q", errUtils.ErrArtifactPackagingInvalid, artifact.KindCatalog, prepared.Record.Kind)
	}
	var (
		payload   *stagedBlob
		documents = map[artifact.DocumentRole]stagedBlob{}
	)
	for index := range prepared.Blobs {
		blob := prepared.Blobs[index]
		switch blob.Role {
		case artifact.BlobPayload:
			if payload != nil {
				return stagedBlob{}, nil, fmt.Errorf("%w: an OCI package has exactly one archive payload blob", errUtils.ErrArtifactPackagingInvalid)
			}
			blob.Descriptor.MediaType = mediaType
			payload = &stagedBlob{blob: blob, layer: layerOf(blob.Descriptor, payloadTitle(prepared.Packaging), "")}
		case artifact.BlobDocument:
			doc, err := documentLayer(&prepared.Record, blob)
			if err != nil {
				return stagedBlob{}, nil, err
			}
			documents[blob.DocumentRole] = doc
		}
	}
	if payload == nil {
		return stagedBlob{}, nil, fmt.Errorf("%w: the prepared artifact has no payload blob", errUtils.ErrArtifactPackagingInvalid)
	}
	ordered, err := orderedDocuments(&prepared.Record, documents)
	return *payload, ordered, err
}

// documentLayer builds the layer of one staged document, requiring it to match the Record.
func documentLayer(record *artifact.Record, blob artifact.StagedBlob) (stagedBlob, error) {
	for index := range record.Documents {
		doc := record.Documents[index]
		if doc.Role != blob.DocumentRole {
			continue
		}
		if doc.Digest != blob.Descriptor.Digest || doc.Size != blob.Descriptor.Size {
			return stagedBlob{}, fmt.Errorf("%w: the staged %s differs from the Record's descriptor", errUtils.ErrArtifactPackagingInvalid, doc.Role)
		}
		blob.Descriptor.MediaType = doc.MediaType
		return stagedBlob{blob: blob, layer: layerOf(blob.Descriptor, path.Base(doc.SourcePath), doc.Role)}, nil
	}
	return stagedBlob{}, fmt.Errorf("%w: a staged %s document is not described by the Record", errUtils.ErrArtifactPackagingInvalid, blob.DocumentRole)
}

// orderedDocuments returns the documents in the fixed README then CHANGELOG order and requires
// every document the Record describes to have been staged.
func orderedDocuments(record *artifact.Record, staged map[artifact.DocumentRole]stagedBlob) ([]stagedBlob, error) {
	var ordered []stagedBlob
	for _, role := range []artifact.DocumentRole{artifact.DocumentReadme, artifact.DocumentChangelog} {
		doc, present := staged[role]
		described := false
		for index := range record.Documents {
			described = described || record.Documents[index].Role == role
		}
		switch {
		case present:
			ordered = append(ordered, doc)
		case described:
			return nil, fmt.Errorf("%w: the Record describes a %s that was not staged", errUtils.ErrArtifactPackagingInvalid, role)
		}
	}
	return ordered, nil
}

func layerOf(desc ocispec.Descriptor, title string, role artifact.DocumentRole) ocispec.Descriptor {
	annotations := map[string]string{annotationTitle: title}
	if role != "" {
		annotations[annotationDocumentRole] = string(role)
	}
	return ocispec.Descriptor{MediaType: desc.MediaType, Digest: desc.Digest, Size: desc.Size, Annotations: annotations}
}

func payloadTitle(packaging artifact.Packaging) string {
	switch packaging {
	case artifact.PackagingTar:
		return "content.tar"
	case artifact.PackagingZip:
		return "content.zip"
	case artifact.PackagingCatalog:
		return "catalog.json"
	default:
		return "content.tar.gz"
	}
}

// manifestAnnotations carries the standard OCI fields for registry-side browsing. They are
// derived from the Record only, never from the clock.
func manifestAnnotations(record *artifact.Record) map[string]string {
	annotations := map[string]string{
		annotationCreated: record.CreatedAt.Time().UTC().Format(time.RFC3339),
		annotationTitle:   record.Name,
		annotationVersion: record.Version,
		annotationKind:    record.Kind,
	}
	if record.Metadata.Description != "" {
		annotations[annotationDescription] = record.Metadata.Description
	}
	return annotations
}

func specVersion() specs.Versioned {
	return specs.Versioned{SchemaVersion: ociSchemaVersion}
}
