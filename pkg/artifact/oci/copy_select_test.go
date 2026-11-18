package oci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Two reference names for one root digest select that root once; different digests stay ambiguous.
func TestLayoutRoot_DeduplicatesNamesOfOneDigest(t *testing.T) {
	first := digest.FromString("first root")
	second := digest.FromString("second root")
	entry := func(dgst digest.Digest, name string) ocispec.Descriptor {
		return ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest, Digest: dgst, Size: 10,
			Annotations: map[string]string{ocispec.AnnotationRefName: name},
		}
	}
	tests := []struct {
		name     string
		entries  []ocispec.Descriptor
		selector string
		want     digest.Digest
		wantErr  error
	}{
		{name: "digest selector over two names", entries: []ocispec.Descriptor{entry(first, "a:1"), entry(first, "b:1")}, selector: first.String(), want: first},
		{name: "no selector over two names", entries: []ocispec.Descriptor{entry(first, "a:1"), entry(first, "b:1")}, want: first},
		{name: "name selector", entries: []ocispec.Descriptor{entry(first, "a:1"), entry(second, "b:1")}, selector: "b:1", want: second},
		{name: "two digests stay ambiguous", entries: []ocispec.Descriptor{entry(first, "a:1"), entry(second, "b:1")}, wantErr: errUtils.ErrArtifactRefInvalid},
		{name: "a selector matching nothing", entries: []ocispec.Descriptor{entry(first, "a:1")}, selector: "missing:1", wantErr: errUtils.ErrArtifactRefInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			raw, err := json.Marshal(ocispec.Index{Versioned: specs.Versioned{SchemaVersion: 2}, MediaType: ocispec.MediaTypeImageIndex, Manifests: tt.entries})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ocispec.ImageIndexFile), raw, 0o600))

			got, err := layoutRoot(dir, tt.selector)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Digest)
		})
	}
}
