package signature

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
)

func TestReferrersRoundTripAndOfflineEvidence(t *testing.T) {
	server := httptest.NewServer(registry.New())
	defer server.Close()
	client := oci.NewClient(nil)
	repo, err := name.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/test/plugin")
	require.NoError(t, err)
	index := empty.Index
	require.NoError(t, client.WriteIndex(t.Context(), repo.Tag("1.0.0"), index))
	descriptor, err := client.Head(t.Context(), repo.Tag("1.0.0"))
	require.NoError(t, err)
	subject := repo.Digest(descriptor.Digest.String())
	found, err := Fetch(t.Context(), client, subject)
	require.NoError(t, err)
	require.Empty(t, found)
	raw := []byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`)
	image, err := mutate.AppendLayers(empty.Image, static.NewLayer(raw, types.MediaType("application/json")))
	require.NoError(t, err)
	image = mutate.MediaType(image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.MediaType("application/vnd.dev.sigstore.bundle.v0.3+json"))
	image = mutate.Subject(image, *descriptor).(v1.Image)
	digest, err := image.Digest()
	require.NoError(t, err)
	require.NoError(t, client.WriteImage(t.Context(), repo.Digest(digest.String()), image))
	found, err = Fetch(t.Context(), client, subject)
	require.NoError(t, err)
	require.Equal(t, [][]byte{raw}, found)
	directory := t.TempDir()
	require.NoError(t, Save(directory, found))
	loaded, err := Load(directory)
	require.NoError(t, err)
	require.Equal(t, found, loaded)
	require.NoError(t, os.Remove(filepath.Join(directory, "signatures.json")))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "signature.sigstore.json"), raw, 0o600))
	loaded, err = Load(directory)
	require.NoError(t, err)
	require.Equal(t, found, loaded)
}

func TestEvidenceRejectsMalformedAndOversizedFiles(t *testing.T) {
	directory := t.TempDir()
	_, err := Load(directory)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorIs(t, Save(directory, [][]byte{[]byte("invalid")}), errUtils.ErrPluginSignatureInvalid)
	require.ErrorIs(t, Save(directory, make([][]byte, MaxBundles+1)), errUtils.ErrPluginPayloadTooLarge)
	require.ErrorIs(t, Save(directory, [][]byte{make([]byte, MaxBundleSize+1)}), errUtils.ErrPluginPayloadTooLarge)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "signatures.json"), []byte("not json"), 0o600))
	_, err = Load(directory)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
}

func TestFetchRejectsUnboundEvidenceAndFailedBlobDownloads(t *testing.T) {
	for _, tc := range []string{"no subject", "different subject", "extra layer", "blob unavailable", "collection too large"} {
		t.Run(tc, func(t *testing.T) {
			var failBlob atomic.Bool
			handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if failBlob.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/blobs/") {
					http.Error(w, "bundle unavailable", http.StatusForbidden)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			client := oci.NewClient(nil)
			repo, err := name.NewRepository(server.Listener.Addr().String() + "/plugin")
			require.NoError(t, err)
			require.NoError(t, client.WriteIndex(t.Context(), repo.Tag("1.0.0"), empty.Index))
			subjectDescriptor, err := client.Head(t.Context(), repo.Tag("1.0.0"))
			require.NoError(t, err)
			subject := repo.Digest(subjectDescriptor.Digest.String())
			raw := []byte("{}")
			if tc == "collection too large" {
				raw = []byte(strings.Repeat(" ", MaxBundleSize))
			}
			layer := static.NewLayer(raw, types.MediaType("application/json"))
			img, err := mutate.AppendLayers(empty.Image, layer)
			require.NoError(t, err)
			img = mutate.MediaType(img, types.OCIManifestSchema1)
			switch tc {
			case "no subject":
			case "different subject":
				img = mutate.Subject(img, v1.Descriptor{Digest: v1.Hash{Algorithm: "sha256", Hex: strings.Repeat("a", 64)}}).(v1.Image)
			default:
				img = mutate.Subject(img, *subjectDescriptor).(v1.Image)
			}
			if tc == "extra layer" {
				img, err = mutate.AppendLayers(img, layer)
				require.NoError(t, err)
			}
			digest, err := img.Digest()
			require.NoError(t, err)
			require.NoError(t, client.WriteImage(t.Context(), repo.Digest(digest.String()), img))
			descriptor, err := client.Get(t.Context(), repo.Digest(digest.String()))
			require.NoError(t, err)
			discovered := descriptor.Descriptor
			discovered.ArtifactType = "application/vnd.dev.sigstore.bundle.v0.3+json"
			items := []v1.Descriptor{discovered}
			if tc == "collection too large" {
				items = []v1.Descriptor{discovered, discovered, discovered, discovered, discovered}
			}
			index := NewMockImageIndex(gomock.NewController(t))
			index.EXPECT().IndexManifest().Return(&v1.IndexManifest{Manifests: items}, nil)
			registry := NewMockReferrerRegistry(gomock.NewController(t))
			registry.EXPECT().Referrers(gomock.Any(), subject).Return(index, nil)
			registry.EXPECT().Get(gomock.Any(), repo.Digest(digest.String())).Return(descriptor, nil).Times(len(items))
			failBlob.Store(tc == "blob unavailable")
			bundles, err := Fetch(t.Context(), registry, subject)
			require.Error(t, err)
			require.Nil(t, bundles, "failed attachment validation must not return partial evidence")
			switch tc {
			case "collection too large":
				require.ErrorIs(t, err, errUtils.ErrPluginPayloadTooLarge)
			case "blob unavailable":
			default:
				require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
			}
		})
	}
}
