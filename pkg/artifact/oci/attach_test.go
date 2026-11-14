//nolint:gocritic // OCI descriptors are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

// referrerStore serves a fixed referrer list and the manifests of the referrers that have content.
// Fetching anything else fails with fetchErr, standing in for a registry that is down.
type referrerStore struct {
	store
	referrers []ocispec.Descriptor
	content   map[digest.Digest][]byte
	fetchErr  error
}

func (s *referrerStore) Predecessors(context.Context, ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	return s.referrers, nil
}

func (s *referrerStore) Fetch(_ context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	raw, ok := s.content[target.Digest]
	if !ok {
		return nil, s.fetchErr
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

// bundleReferrer builds a valid bundle-typed referrer of root and its descriptor.
func bundleReferrer(t *testing.T, root ocispec.Descriptor) (ocispec.Descriptor, []byte) {
	t.Helper()
	bundleType := bundleMediaTypePrefix + "v0.3+json"
	manifest := ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: bundleType,
		Config:       ocispec.DescriptorEmptyJSON,
		Layers:       []ocispec.Descriptor{{MediaType: bundleType, Digest: digest.FromString("bundle"), Size: 6}},
		Subject:      &root,
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	return ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, ArtifactType: bundleType, Digest: digest.FromBytes(raw), Size: int64(len(raw))}, raw
}

// An oversized referrer is malformed evidence: it is reported, and it cannot block a valid
// signature from another referrer. Any other fetch failure stays fatal.
func TestAttachments_OversizedReferrerIsMalformedEvidence(t *testing.T) {
	root := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("root"), Size: 100}
	valid, validRaw := bundleReferrer(t, root)
	oversized := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest, ArtifactType: bundleMediaTypePrefix + "v0.3+json",
		Digest: digest.FromString("oversized"), Size: maxManifestSize + 1,
	}
	tests := []struct {
		name        string
		referrers   []ocispec.Descriptor
		fetchErr    error
		wantValid   []digest.Digest
		wantProblem string
		wantErr     error
	}{
		{
			name: "oversized referrer beside a valid one", referrers: []ocispec.Descriptor{oversized, valid},
			wantValid: []digest.Digest{valid.Digest}, wantProblem: "too large",
		},
		{
			name: "only an oversized referrer", referrers: []ocispec.Descriptor{oversized},
			wantProblem: "too large",
		},
		{
			name: "a registry failure stays fatal", referrers: []ocispec.Descriptor{{
				MediaType: ocispec.MediaTypeImageManifest, ArtifactType: bundleMediaTypePrefix + "v0.3+json", Digest: digest.FromString("down"), Size: 10,
			}},
			fetchErr: errors.New("registry unavailable"), wantErr: errUtils.ErrArtifactDownloadFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &referrerStore{referrers: tt.referrers, content: map[digest.Digest][]byte{valid.Digest: validRaw}, fetchErr: tt.fetchErr}
			set, err := (&Repository{cache: map[string][]byte{}}).attachments(context.Background(), st, "team/app", root)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			var got []digest.Digest
			for _, item := range set.valid {
				got = append(got, item.manifestDesc.Digest)
			}
			assert.Equal(t, tt.wantValid, got)
			require.Len(t, set.diagnostics, 1)
			assert.Equal(t, "malformed-evidence", set.diagnostics[0].Code)
			assert.Equal(t, artifact.SeverityWarning, set.diagnostics[0].Severity)
			assert.Contains(t, set.diagnostics[0].Message, tt.wantProblem)
			assert.Contains(t, set.diagnostics[0].Message, oversized.Digest.String())
		})
	}
}

// A referrer whose manifest bytes fail digest verification, or whose manifest blob is gone, is
// malformed evidence like an oversized one: it is reported and cannot block a valid signature
// carried by another referrer.
func TestAttachments_IntegrityFailedReferrerIsMalformedEvidence(t *testing.T) {
	root := ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.FromString("root"), Size: 100}
	valid, validRaw := bundleReferrer(t, root)
	bundleType := bundleMediaTypePrefix + "v0.3+json"
	corrupt := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest, ArtifactType: bundleType,
		Digest: digest.FromString("corrupt"), Size: int64(len("tampered bytes")),
	}
	missing := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageManifest, ArtifactType: bundleType,
		Digest: digest.FromString("missing"), Size: 10,
	}
	tests := []struct {
		name      string
		referrers []ocispec.Descriptor
		bad       ocispec.Descriptor
		wantValid []digest.Digest
		wantText  string
	}{
		{name: "corrupt referrer beside a valid one", referrers: []ocispec.Descriptor{corrupt, valid}, bad: corrupt, wantValid: []digest.Digest{valid.Digest}, wantText: "integrity"},
		{name: "only a corrupt referrer", referrers: []ocispec.Descriptor{corrupt}, bad: corrupt, wantText: "integrity"},
		{name: "missing referrer blob beside a valid one", referrers: []ocispec.Descriptor{missing, valid}, bad: missing, wantValid: []digest.Digest{valid.Digest}, wantText: "integrity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &referrerStore{
				referrers: tt.referrers,
				content: map[digest.Digest][]byte{
					valid.Digest:   validRaw,
					corrupt.Digest: []byte("tampered bytes"),
				},
				fetchErr: errdef.ErrNotFound,
			}
			set, err := (&Repository{cache: map[string][]byte{}}).attachments(context.Background(), st, "team/app", root)
			require.NoError(t, err)
			var got []digest.Digest
			for _, item := range set.valid {
				got = append(got, item.manifestDesc.Digest)
			}
			assert.Equal(t, tt.wantValid, got)
			require.Len(t, set.diagnostics, 1)
			assert.Equal(t, "malformed-evidence", set.diagnostics[0].Code)
			assert.Equal(t, artifact.SeverityWarning, set.diagnostics[0].Severity)
			assert.Contains(t, set.diagnostics[0].Message, tt.wantText)
			assert.Contains(t, set.diagnostics[0].Message, tt.bad.Digest.String())
		})
	}
}
