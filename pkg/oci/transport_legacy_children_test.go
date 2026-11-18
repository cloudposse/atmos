package oci

import (
	"bytes"
	"io"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// An index child that is an image with a streaming layer has no digest until the stream is read once, so
// every child is prepared before the index manifest, whose descriptors depend on it, is read.
func TestLegacyIndexGraphPreparesStreamingChildrenBeforeTheIndexManifest(t *testing.T) {
	payload := bytes.Repeat([]byte("streamed layer bytes "), 64)
	image, err := mutate.AppendLayers(empty.Image, stream.NewLayer(io.NopCloser(bytes.NewReader(payload))))
	require.NoError(t, err)
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: image})

	graph, err := legacyIndexGraph(t.Context(), index)
	require.NoError(t, err)
	t.Cleanup(graph.close)

	raw, err := index.RawManifest()
	require.NoError(t, err, "the index manifest exists once its children were prepared")
	assert.Equal(t, digest.FromBytes(raw), graph.root.Digest, "the root describes the manifest the index now serves")
	layers, err := image.Layers()
	require.NoError(t, err)
	hash, err := layers[0].Digest()
	require.NoError(t, err)
	exists, err := graph.Exists(t.Context(), ocispec.Descriptor{Digest: digest.Digest(hash.String())})
	require.NoError(t, err)
	assert.True(t, exists, "the streamed layer is served from its spool")
}

// A child that is a layer rather than an image or an index is a blob the registry must receive too.
func TestLegacyIndexGraphServesLayerChildren(t *testing.T) {
	blob := []byte("an artifact blob that is neither an image nor an index")
	layer := static.NewLayer(blob, types.MediaType("application/vnd.example.blob.v1"))
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: layer})

	graph, err := legacyIndexGraph(t.Context(), index)
	require.NoError(t, err)
	t.Cleanup(graph.close)

	hash, err := layer.Digest()
	require.NoError(t, err)
	reader, err := graph.Fetch(t.Context(), ocispec.Descriptor{Digest: digest.Digest(hash.String())})
	require.NoError(t, err)
	defer reader.Close()
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, blob, got)
}

// Schema 1 images have no manifest the legacy graph can carry as OCI content, so they are refused with an
// explicit error instead of failing later in Manifest().
func TestLegacyImageGraphRefusesSchemaOneImages(t *testing.T) {
	image, err := random.Image(64, 1)
	require.NoError(t, err)
	for _, mediaType := range []types.MediaType{types.DockerManifestSchema1, types.DockerManifestSchema1Signed} {
		t.Run(string(mediaType), func(t *testing.T) {
			graph, err := legacyImageGraph(t.Context(), mutate.MediaType(image, mediaType))

			require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
			assert.Contains(t, err.Error(), "schema 1")
			assert.Nil(t, graph)
		})
	}
}
