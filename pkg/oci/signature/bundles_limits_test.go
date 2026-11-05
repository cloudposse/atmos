package signature

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

//go:generate go run go.uber.org/mock/mockgen -destination=registry_mock_test.go -package=signature github.com/cloudposse/atmos/pkg/oci ReferrerRegistry
//go:generate go run go.uber.org/mock/mockgen -destination=index_mock_test.go -package=signature github.com/google/go-containerregistry/pkg/v1 ImageIndex

func TestEvidenceCollectionReadAndWriteLimits(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{"too many bundles", []byte("[" + strings.Repeat("{},", MaxBundles) + "{}]")},
		{"individual bundle too large", []byte(`["` + strings.Repeat("a", MaxBundleSize) + `"]`)},
		{"collection too large", bytes.Repeat([]byte(" "), maxCollectionSize+1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, bundleFile), tc.raw, 0o600))
			result, err := Load(directory)
			require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
			require.Nil(t, result)
		})
	}
	directory := t.TempDir()
	large := []byte(`"` + strings.Repeat("a", MaxBundleSize-2) + `"`)
	require.ErrorIs(t, Save(directory, [][]byte{large, large, large, large}), errUtils.ErrPluginPayloadTooLarge)
	_, err := os.Stat(filepath.Join(directory, bundleFile))
	require.ErrorIs(t, err, os.ErrNotExist, "failed saves must not leave partial evidence")
}

func TestEvidenceIOFailuresCannotFallBackToAnotherFile(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(directory, bundleFile), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "signature.sigstore.json"), []byte("{}"), 0o600))
	bundles, err := Load(directory)
	require.Error(t, err, "an unreadable collection must not fall back to another signature")
	require.Nil(t, bundles)
	require.Error(t, Save(directory, [][]byte{[]byte("{}")}))
}

func TestFetchPropagatesDiscoveryAndManifestFailures(t *testing.T) {
	subject, err := name.NewDigest("registry.example.com/plugin@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	registry := NewMockReferrerRegistry(gomock.NewController(t))
	registry.EXPECT().Referrers(gomock.Any(), subject).Return(nil, os.ErrPermission)
	bundles, err := Fetch(t.Context(), registry, subject)
	require.ErrorIs(t, err, os.ErrPermission)
	require.Nil(t, bundles)
	index := NewMockImageIndex(gomock.NewController(t))
	registry.EXPECT().Referrers(gomock.Any(), subject).Return(index, nil)
	index.EXPECT().IndexManifest().Return(nil, os.ErrInvalid)
	bundles, err = Fetch(t.Context(), registry, subject)
	require.ErrorIs(t, err, os.ErrInvalid)
	require.Nil(t, bundles)
}

func TestFetchBoundsDescriptorsAndIgnoresOtherArtifactTypes(t *testing.T) {
	subject, err := name.NewDigest("registry.example.com/plugin@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	for _, tc := range []struct {
		name  string
		items []v1.Descriptor
		want  error
	}{
		{"bounded discovery", make([]v1.Descriptor, MaxBundles+1), errUtils.ErrPluginPayloadTooLarge},
		{"unrelated artifacts", []v1.Descriptor{{ArtifactType: "application/vnd.example.sbom"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewMockReferrerRegistry(gomock.NewController(t))
			index := NewMockImageIndex(gomock.NewController(t))
			registry.EXPECT().Referrers(gomock.Any(), subject).Return(index, nil)
			index.EXPECT().IndexManifest().Return(&v1.IndexManifest{Manifests: tc.items}, nil)
			bundles, err := Fetch(t.Context(), registry, subject)
			require.ErrorIs(t, err, tc.want)
			require.Empty(t, bundles)
		})
	}
}

func TestFetchRejectsManifestDownloadIntegrityAndMediaTypeFailures(t *testing.T) {
	subject, err := name.NewDigest("registry.example.com/plugin@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	raw := []byte("{}")
	hash, size, err := v1.SHA256(bytes.NewReader(raw))
	require.NoError(t, err)
	for _, tc := range []struct {
		name string
		get  *remote.Descriptor
		err  error
		want error
	}{
		{"download fails", nil, os.ErrPermission, os.ErrPermission},
		{"changed manifest", &remote.Descriptor{Manifest: []byte("changed")}, nil, errUtils.ErrOCIIntegrity},
		{"not an image", &remote.Descriptor{Manifest: raw, Descriptor: v1.Descriptor{MediaType: types.OCIImageIndex}}, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewMockReferrerRegistry(gomock.NewController(t))
			index := NewMockImageIndex(gomock.NewController(t))
			index.EXPECT().IndexManifest().Return(&v1.IndexManifest{Manifests: []v1.Descriptor{
				{ArtifactType: "application/vnd.dev.sigstore.bundle.v0.3+json", Digest: hash, Size: size},
			}}, nil)
			registry.EXPECT().Referrers(gomock.Any(), subject).Return(index, nil)
			registry.EXPECT().Get(gomock.Any(), subject.Context().Digest(hash.String())).Return(tc.get, tc.err)
			bundles, err := Fetch(t.Context(), registry, subject)
			require.Error(t, err)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}
			require.Nil(t, bundles)
		})
	}
}

func TestEvidenceCollectionRoundTripPreservesDistinctBundles(t *testing.T) {
	directory := t.TempDir()
	input := [][]byte{[]byte(`{"publisher":"one"}`), []byte(`{"publisher":"two"}`)}
	require.NoError(t, Save(directory, input))
	bundles, err := Load(directory)
	require.NoError(t, err)
	require.Equal(t, input, bundles)
	var stored []json.RawMessage
	raw, err := os.ReadFile(filepath.Join(directory, bundleFile))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &stored))
	require.JSONEq(t, string(input[0]), string(stored[0]))
	require.JSONEq(t, string(input[1]), string(stored[1]))
}

func TestFetchRejectsMalformedManifestEvenWithMatchingDigest(t *testing.T) {
	subject, err := name.NewDigest("registry.example.com/plugin@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	raw := []byte("{")
	hash, size, err := v1.SHA256(bytes.NewReader(raw))
	require.NoError(t, err)
	descriptor := v1.Descriptor{MediaType: types.OCIManifestSchema1, ArtifactType: "application/vnd.dev.sigstore.bundle.v0.3+json", Digest: hash, Size: size}
	index := NewMockImageIndex(gomock.NewController(t))
	index.EXPECT().IndexManifest().Return(&v1.IndexManifest{Manifests: []v1.Descriptor{descriptor}}, nil)
	registry := NewMockReferrerRegistry(gomock.NewController(t))
	registry.EXPECT().Referrers(gomock.Any(), subject).Return(index, nil)
	registry.EXPECT().Get(gomock.Any(), subject.Context().Digest(hash.String())).Return(&remote.Descriptor{Descriptor: descriptor, Manifest: raw}, nil)
	bundles, err := Fetch(t.Context(), registry, subject)
	require.Error(t, err)
	require.Nil(t, bundles, "digest integrity alone must not make malformed metadata usable")
}
