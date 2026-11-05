//nolint:gocritic,lintroller // Descriptor-by-value parameters and thin store delegators mirror the ORAS content contracts so stores stay interchangeable.
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/errdef"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// legacyRoot is the only reference a legacyGraph resolves.
const legacyRoot = "root"

// legacyGraph exposes the raw bytes of a go-containerregistry image or index as a
// read-only OCI graph. It serializes exactly what the legacy object already holds:
// manifests are never rebuilt (no ORAS PackManifest), so digests cannot drift.
type legacyGraph struct {
	root  ocispec.Descriptor
	blobs map[digest.Digest]func() (io.ReadCloser, error)
}

var _ ReadOnlyStore = (*legacyGraph)(nil)

func newLegacyGraph() *legacyGraph {
	return &legacyGraph{blobs: make(map[digest.Digest]func() (io.ReadCloser, error))}
}

func legacyImageGraph(image v1.Image) (*legacyGraph, error) {
	graph := newLegacyGraph()
	root, err := graph.addImage(image)
	if err != nil {
		return nil, err
	}
	graph.root = root
	return graph, nil
}

func legacyIndexGraph(index v1.ImageIndex) (*legacyGraph, error) {
	graph := newLegacyGraph()
	root, err := graph.addIndex(index)
	if err != nil {
		return nil, err
	}
	graph.root = root
	return graph, nil
}

func (g *legacyGraph) addBytes(mediaType string, raw []byte) ocispec.Descriptor {
	desc := Descriptor(mediaType, raw)
	g.blobs[desc.Digest] = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }
	return desc
}

func (g *legacyGraph) addImage(image v1.Image) (ocispec.Descriptor, error) {
	raw, err := image.RawManifest()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	mediaType, err := image.MediaType()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	manifest, err := image.Manifest()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	rawConfig, err := image.RawConfigFile()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	g.blobs[digest.Digest(manifest.Config.Digest.String())] = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(rawConfig)), nil
	}
	for i := range manifest.Layers {
		hash := manifest.Layers[i].Digest
		g.blobs[digest.Digest(hash.String())] = func() (io.ReadCloser, error) {
			layer, err := image.LayerByDigest(hash)
			if err != nil {
				return nil, err
			}
			return layer.Compressed()
		}
	}
	return g.addBytes(string(mediaType), raw), nil
}

func (g *legacyGraph) addIndex(index v1.ImageIndex) (ocispec.Descriptor, error) {
	raw, err := index.RawManifest()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	mediaType, err := index.MediaType()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return ocispec.Descriptor{}, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	for i := range manifest.Manifests {
		if err := g.addChild(index, &manifest.Manifests[i]); err != nil {
			return ocispec.Descriptor{}, err
		}
	}
	return g.addBytes(string(mediaType), raw), nil
}

// addChild records one child manifest or nested index of a legacy index.
func (g *legacyGraph) addChild(index v1.ImageIndex, child *v1.Descriptor) error {
	switch {
	case child.MediaType.IsIndex():
		nested, err := index.ImageIndex(child.Digest)
		if err != nil {
			return errors.Join(errUtils.ErrOCIIntegrity, err)
		}
		_, err = g.addIndex(nested)
		return err
	case child.MediaType.IsImage():
		nested, err := index.Image(child.Digest)
		if err != nil {
			return errors.Join(errUtils.ErrOCIIntegrity, err)
		}
		_, err = g.addImage(nested)
		return err
	default:
		return fmt.Errorf("%w: unsupported child media type %q", errUtils.ErrOCIIntegrity, child.MediaType)
	}
}

// Fetch streams a known blob.
func (g *legacyGraph) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	open, found := g.blobs[target.Digest]
	if !found {
		return nil, fmt.Errorf("%s: %w", target.Digest, errdef.ErrNotFound)
	}
	return open()
}

// Exists reports whether the legacy object holds the blob.
func (g *legacyGraph) Exists(_ context.Context, target ocispec.Descriptor) (bool, error) {
	_, found := g.blobs[target.Digest]
	return found, nil
}

// Resolve returns the root for the only reference the graph knows.
func (g *legacyGraph) Resolve(_ context.Context, reference string) (ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.legacyGraph.Resolve")()

	if reference != legacyRoot && reference != g.root.Digest.String() {
		return ocispec.Descriptor{}, fmt.Errorf("%s: %w", reference, errdef.ErrNotFound)
	}
	return g.root, nil
}

// referrersIndex presents ORAS referrer descriptors as the legacy image index.
type referrersIndex struct {
	manifest *v1.IndexManifest
	raw      []byte
	hash     v1.Hash
}

var _ v1.ImageIndex = (*referrersIndex)(nil)

func newReferrersIndex(found []ocispec.Descriptor) (*referrersIndex, error) {
	manifest := &v1.IndexManifest{SchemaVersion: 2, MediaType: types.OCIImageIndex, Manifests: make([]v1.Descriptor, 0, len(found))}
	for i := range found {
		hash, err := v1.NewHash(found[i].Digest.String())
		if err != nil {
			return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
		}
		manifest.Manifests = append(manifest.Manifests, v1.Descriptor{
			MediaType: types.MediaType(found[i].MediaType), Size: found[i].Size, Digest: hash,
			Annotations: found[i].Annotations, ArtifactType: found[i].ArtifactType, URLs: found[i].URLs,
		})
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	hash, _, err := v1.SHA256(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	return &referrersIndex{manifest: manifest, raw: raw, hash: hash}, nil
}

//nolint:lintroller // Trivial getters of an in-memory value.
func (r *referrersIndex) MediaType() (types.MediaType, error) { return types.OCIImageIndex, nil }

//nolint:lintroller // Trivial getters of an in-memory value.
func (r *referrersIndex) Digest() (v1.Hash, error) { return r.hash, nil }

//nolint:lintroller // Trivial getters of an in-memory value.
func (r *referrersIndex) Size() (int64, error) { return int64(len(r.raw)), nil }

//nolint:lintroller // Trivial getters of an in-memory value.
func (r *referrersIndex) IndexManifest() (*v1.IndexManifest, error) { return r.manifest, nil }

//nolint:lintroller // Trivial getters of an in-memory value.
func (r *referrersIndex) RawManifest() ([]byte, error) { return r.raw, nil }

// Image is unsupported: a referrers response holds descriptors, not content.
//
//nolint:lintroller // Constant error.
func (r *referrersIndex) Image(v1.Hash) (v1.Image, error) {
	return nil, errUtils.ErrOCIRegistry
}

// ImageIndex is unsupported: a referrers response holds descriptors, not content.
//
//nolint:lintroller // Constant error.
func (r *referrersIndex) ImageIndex(v1.Hash) (v1.ImageIndex, error) {
	return nil, errUtils.ErrOCIRegistry
}
