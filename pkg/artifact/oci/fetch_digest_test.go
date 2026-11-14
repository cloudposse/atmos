package oci

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// A registry manifest can list a layer or child with an empty or malformed digest. The reader must
// report it as an integrity failure; the digest library panics on a digest without an algorithm.
func TestMalformedDescriptorDigestsAreIntegrityFailures(t *testing.T) {
	ctx := context.Background()
	malformed := []digest.Digest{"", "sha256", "sha256:zz", digest.Digest("unknown:" + digest.FromString("x").Encoded())}

	for _, bad := range malformed {
		t.Run("lazy reader "+string(bad), func(t *testing.T) {
			reader := newLazyReader(ctx, nil, ocispec.Descriptor{MediaType: artifact.MediaTypeTar, Digest: bad, Size: 3})
			_, err := io.ReadAll(reader)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})

		t.Run("manifest layer "+string(bad), func(t *testing.T) {
			raw, err := json.Marshal(ocispec.Manifest{
				Config: ocispec.Descriptor{MediaType: artifact.MediaTypeConfig, Digest: digest.FromString("c"), Size: 1},
				Layers: []ocispec.Descriptor{{MediaType: artifact.MediaTypeTar, Digest: bad, Size: 3}},
			})
			require.NoError(t, err)
			desc := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromBytes(raw), Size: int64(len(raw))}

			_, err = classify(desc, raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
			_, err = successorsOf(desc, raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})

		t.Run("index child "+string(bad), func(t *testing.T) {
			raw, err := json.Marshal(ocispec.Index{Manifests: []ocispec.Descriptor{{MediaType: ocispec.MediaTypeImageManifest, Digest: bad, Size: 3}}})
			require.NoError(t, err)
			desc := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageIndex, Digest: digest.FromBytes(raw), Size: int64(len(raw))}

			_, err = classify(desc, raw)
			require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
		})
	}

	t.Run("well-formed digests classify", func(t *testing.T) {
		raw, err := json.Marshal(ocispec.Manifest{
			Config: ocispec.Descriptor{MediaType: artifact.MediaTypeConfig, Digest: digest.FromString("c"), Size: 1},
			Layers: []ocispec.Descriptor{{MediaType: artifact.MediaTypeTar, Digest: digest.FromString("l"), Size: 1}},
		})
		require.NoError(t, err)
		loaded, err := classify(ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromBytes(raw), Size: int64(len(raw))}, raw)
		require.NoError(t, err)
		assert.Equal(t, classAtmos, loaded.class)
	})
}
