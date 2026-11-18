package oci

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// The media types a root is classified by are the ones the copy transport can traverse. ORAS follows
// a manifest to its config and layers only for these exact types, so a type that merely ends the way
// one of them does would be copied as a leaf: an import would tag the root without its content.
var traversableRoots = []struct {
	name      string
	mediaType string
	index     bool
}{
	{name: "OCI image manifest", mediaType: ocispec.MediaTypeImageManifest},
	{name: "Docker image manifest", mediaType: "application/vnd.docker.distribution.manifest.v2+json"},
	{name: "OCI image index", mediaType: ocispec.MediaTypeImageIndex, index: true},
	{name: "Docker manifest list", mediaType: "application/vnd.docker.distribution.manifest.list.v2+json", index: true},
}

var lookalikeRoots = []struct {
	name      string
	mediaType string
}{
	{name: "custom manifest suffix", mediaType: "application/x-custom.manifest.v2+json"},
	{name: "custom manifest list suffix", mediaType: "application/x-custom.manifest.list.v2+json"},
	{name: "vendor docker-like manifest", mediaType: "application/vnd.example.distribution.manifest.v2+json"},
	{name: "traversable type with a suffix", mediaType: ocispec.MediaTypeImageManifest + "+x"},
	{name: "traversable type in another case", mediaType: "Application/Vnd.Oci.Image.Manifest.V1+Json"},
	{name: "empty", mediaType: ""},
}

func TestRootMediaTypes_AreExactlyTheTraversableOnes(t *testing.T) {
	for _, tt := range traversableRoots {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.index, isIndexMediaType(tt.mediaType))
			assert.Equal(t, !tt.index, isManifestMediaType(tt.mediaType))
		})
	}
	for _, tt := range lookalikeRoots {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, isIndexMediaType(tt.mediaType), "the copy transport would not traverse it")
			assert.False(t, isManifestMediaType(tt.mediaType), "the copy transport would not traverse it")
		})
	}
}

func TestClassify_RejectsMediaTypesTheCopyTransportCannotTraverse(t *testing.T) {
	raw, err := json.Marshal(ocispec.Manifest{
		Config: ocispec.Descriptor{MediaType: artifact.MediaTypeConfig, Digest: digest.FromString("c"), Size: 1},
		Layers: []ocispec.Descriptor{{MediaType: artifact.MediaTypeTar, Digest: digest.FromString("l"), Size: 1}},
	})
	require.NoError(t, err)
	rawIndex, err := json.Marshal(ocispec.Index{Manifests: []ocispec.Descriptor{{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("m"), Size: 1}}})
	require.NoError(t, err)

	for _, tt := range traversableRoots {
		t.Run("classifies "+tt.name, func(t *testing.T) {
			body := raw
			if tt.index {
				body = rawIndex
			}
			loaded, err := classify(ocispec.Descriptor{MediaType: tt.mediaType, Digest: digest.FromBytes(body), Size: int64(len(body))}, body)
			require.NoError(t, err)
			assert.Equal(t, tt.index, loaded.isIndex())
			successors, err := successorsOf(loaded.desc, body)
			require.NoError(t, err)
			assert.NotEmpty(t, successors)
		})
	}
	for _, tt := range lookalikeRoots {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			_, err := classify(ocispec.Descriptor{MediaType: tt.mediaType, Digest: digest.FromBytes(raw), Size: int64(len(raw))}, raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactRecordInvalid)
		})
	}
}

// A root by digest alone has no media type until its bytes say which it is. A manifest that declares a
// type the copy transport cannot traverse is not recovered as one.
func TestManifestTypeOf_RecoversOnlyTraversableTypes(t *testing.T) {
	for _, tt := range traversableRoots {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"mediaType": tt.mediaType})
			require.NoError(t, err)
			got, err := manifestTypeOf(digest.FromBytes(raw), raw)
			require.NoError(t, err)
			assert.Equal(t, tt.mediaType, got)
		})
	}
	for _, tt := range lookalikeRoots {
		if tt.mediaType == "" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"mediaType": tt.mediaType, "config": map[string]any{}})
			require.NoError(t, err)
			got, err := manifestTypeOf(digest.FromBytes(raw), raw)
			require.NoError(t, err)
			assert.Empty(t, got, "a declared type that is not traversable stays rejected")
		})
	}
}

// The graph source the signing and reading paths use follows the same set of types, so it ends where
// the copy transport ends.
func TestGraphSource_DoesNotFollowLookalikeRoots(t *testing.T) {
	for _, tt := range lookalikeRoots {
		t.Run(tt.name, func(t *testing.T) {
			source := &graphSource{}
			successors, err := source.Successors(context.Background(), ocispec.Descriptor{MediaType: tt.mediaType, Digest: digest.FromString("x"), Size: 1})
			require.NoError(t, err)
			assert.Empty(t, successors)
		})
	}
}
