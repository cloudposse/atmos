//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
)

// rootClass is what a resolved root is. The classes never overlap.
type rootClass int

const (
	// Atmos package whose config blob is the canonical Record.
	classAtmos rootClass = iota

	// Legacy Atmos plugin release, immutable and never a generic artifact.
	classPlugin

	// Foreign single image or artifact manifest.
	classImage

	// Foreign image index, for example a multi-platform image.
	classIndex
)

// String names the class for diagnostics.
func (c rootClass) String() string {
	defer perf.Track(nil, "oci.rootClass.String")()

	switch c {
	case classAtmos:
		return "atmos-package"
	case classPlugin:
		return "plugin-release"
	case classImage:
		return "foreign-image"
	case classIndex:
		return "foreign-index"
	default:
		return "unknown"
	}
}

// A root is one resolved and classified manifest or index. The raw field holds the exact bytes whose digest
// is desc.Digest, so a foreign root is always handled byte-identically.
type root struct {
	class    rootClass
	desc     ocispec.Descriptor
	raw      []byte
	manifest ocispec.Manifest
	index    ocispec.Index
}

// original returns the descriptor of a foreign root with the root-level facts a policy needs
// (artifact type and annotations), so inspection carries them without an Atmos config blob.
func (r *root) original() *ocispec.Descriptor {
	desc := r.desc
	switch {
	case r.isIndex():
		desc.Annotations = r.index.Annotations
		desc.ArtifactType = r.index.ArtifactType
	default:
		desc.Annotations = r.manifest.Annotations
		desc.ArtifactType = r.manifest.ArtifactType
	}
	return &desc
}

// isIndex reports whether the root is an index or manifest list.
func (r *root) isIndex() bool { return isIndexMediaType(r.desc.MediaType) }

// The Docker media types of a manifest and of a manifest list. Together with the OCI types they are the
// whole set of roots the copy transport follows to their config, layers, and children.
const (
	mediaTypeDockerManifest     = "application/vnd.docker.distribution.manifest.v2+json"
	mediaTypeDockerManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
)

// isIndexMediaType reports whether the media type is an OCI index or a Docker manifest list. The match
// is exact: the copy transport follows a root's children only for these types, so a type that merely
// ends the same way would be classified here but copied as a leaf, without its content.
func isIndexMediaType(mediaType string) bool {
	return mediaType == ocispec.MediaTypeImageIndex || mediaType == mediaTypeDockerManifestList
}

// isManifestMediaType reports whether the media type is an OCI image manifest or a Docker manifest.
// The match is exact for the reason isIndexMediaType gives.
func isManifestMediaType(mediaType string) bool {
	return mediaType == ocispec.MediaTypeImageManifest || mediaType == mediaTypeDockerManifest
}

// loadRoot fetches a root by its resolved descriptor with a bounded, digest-verified read and
// classifies it. The class is derived only from the root itself: the config media type of an
// image manifest, or the release annotations of an index.
func loadRoot(ctx context.Context, st atmosoci.Fetcher, desc ocispec.Descriptor) (*root, error) {
	raw, err := atmosoci.FetchBytes(ctx, st, desc, maxManifestSize)
	if err != nil {
		return nil, fmt.Errorf("%w: read manifest %s: %w", errUtils.ErrArtifactDownloadFailed, desc.Digest, err)
	}
	return classify(desc, raw)
}

func classify(desc ocispec.Descriptor, raw []byte) (*root, error) {
	loaded := &root{desc: desc, raw: raw}
	switch {
	case isIndexMediaType(desc.MediaType):
		if err := classifyIndex(loaded); err != nil {
			return nil, err
		}
	case isManifestMediaType(desc.MediaType):
		if err := classifyManifest(loaded); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: %s has unsupported media type %q", errUtils.ErrArtifactRecordInvalid, desc.Digest, desc.MediaType)
	}
	return loaded, nil
}

// classifyIndex decodes an index root and sets its class from the release annotations.
func classifyIndex(loaded *root) error {
	if err := json.Unmarshal(loaded.raw, &loaded.index); err != nil {
		return fmt.Errorf("%w: index %s: %w", errUtils.ErrArtifactIntegrityFailed, loaded.desc.Digest, err)
	}
	if err := requireDescriptorDigests(loaded.desc, loaded.index.Manifests); err != nil {
		return err
	}
	loaded.class = classIndex
	if hasPluginAnnotations(loaded.index.Annotations) {
		loaded.class = classPlugin
	}
	return nil
}

// classifyManifest decodes an image manifest root and sets its class from the config media type.
func classifyManifest(loaded *root) error {
	if err := json.Unmarshal(loaded.raw, &loaded.manifest); err != nil {
		return fmt.Errorf("%w: manifest %s: %w", errUtils.ErrArtifactIntegrityFailed, loaded.desc.Digest, err)
	}
	if err := requireDescriptorDigests(loaded.desc, []ocispec.Descriptor{loaded.manifest.Config}, loaded.manifest.Layers); err != nil {
		return err
	}
	loaded.class = classImage
	switch loaded.manifest.Config.MediaType {
	case artifact.MediaTypeConfig:
		loaded.class = classAtmos
	case pluginConfigMediaType:
		loaded.class = classPlugin
	}
	return nil
}

// requireDescriptorDigests rejects a root that references a descriptor whose digest is malformed. A
// registry serves whatever bytes it holds, so an empty or truncated digest is an integrity failure
// of the root, caught here before a traversal or a read can ask the digest library for it.
func requireDescriptorDigests(root ocispec.Descriptor, groups ...[]ocispec.Descriptor) error {
	for _, group := range groups {
		for index := range group {
			if err := group[index].Digest.Validate(); err != nil {
				return fmt.Errorf("%w: %s references a descriptor with the malformed digest %q: %w", errUtils.ErrArtifactIntegrityFailed, root.Digest, group[index].Digest, err)
			}
		}
	}
	return nil
}

// hasPluginAnnotations reports whether annotations identify a legacy plugin release.
func hasPluginAnnotations(annotations map[string]string) bool {
	if annotations[pluginAnnotationName] != "" {
		return true
	}
	for key := range annotations {
		if strings.HasPrefix(key, pluginPrefix) {
			return true
		}
	}
	return false
}

// packagingOf maps a payload layer media type onto the packaging it implies.
func packagingOf(mediaType string) (artifact.Packaging, bool) {
	switch mediaType {
	case artifact.MediaTypeTar:
		return artifact.PackagingTar, true
	case artifact.MediaTypeTarGzip:
		return artifact.PackagingTgz, true
	case artifact.MediaTypeZip:
		return artifact.PackagingZip, true
	default:
		return "", false
	}
}

// mediaTypeOf maps a packaging onto its PRD payload media type.
func mediaTypeOf(packaging artifact.Packaging) (string, bool) {
	switch packaging {
	case artifact.PackagingTar:
		return artifact.MediaTypeTar, true
	case artifact.PackagingTgz:
		return artifact.MediaTypeTarGzip, true
	case artifact.PackagingZip:
		return artifact.MediaTypeZip, true
	default:
		return "", false
	}
}

// atmosLayers splits an Atmos package's layers into payload and document layers and checks that
// they form exactly the shape this package writes: one archive payload plus role-tagged
// Markdown documents. Anything else is an integrity failure, never a guess.
type atmosLayers struct {
	packaging artifact.Packaging
	payload   []ocispec.Descriptor
	documents map[artifact.DocumentRole]ocispec.Descriptor
}

func splitAtmosLayers(manifest *ocispec.Manifest) (*atmosLayers, error) {
	out := &atmosLayers{documents: map[artifact.DocumentRole]ocispec.Descriptor{}}
	for _, layer := range manifest.Layers {
		if packaging, ok := packagingOf(layer.MediaType); ok {
			if len(out.payload) > 0 {
				return nil, fmt.Errorf("%w: an Atmos package has exactly one archive payload layer", errUtils.ErrArtifactIntegrityFailed)
			}
			out.packaging = packaging
			out.payload = append(out.payload, layer)
			continue
		}
		role := artifact.DocumentRole(layer.Annotations[annotationDocumentRole])
		if layer.MediaType != artifact.DocumentMediaType || (role != artifact.DocumentReadme && role != artifact.DocumentChangelog) {
			return nil, fmt.Errorf("%w: layer %s (%s) is neither the archive payload nor a README or CHANGELOG document", errUtils.ErrArtifactIntegrityFailed, layer.Digest, layer.MediaType)
		}
		if _, duplicate := out.documents[role]; duplicate {
			return nil, fmt.Errorf("%w: duplicate %s document layer", errUtils.ErrArtifactIntegrityFailed, role)
		}
		out.documents[role] = layer
	}
	if len(out.payload) != 1 {
		return nil, fmt.Errorf("%w: an Atmos package has exactly one archive payload layer, found %d", errUtils.ErrArtifactIntegrityFailed, len(out.payload))
	}
	return out, nil
}
