package oci

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
)

func TestReferrersReadsOCI11Discovery(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	hash, err := v1.NewHash("sha256:" + strings.Repeat("b", 64))
	require.NoError(t, err)
	descriptor := v1.Descriptor{
		Digest: hash, Size: 1234, MediaType: types.OCIManifestSchema1,
		ArtifactType: "application/vnd.dev.sigstore.bundle.v0.3+json",
	}
	index := v1.IndexManifest{
		SchemaVersion: 2, MediaType: types.OCIImageIndex,
		Manifests: []v1.Descriptor{descriptor},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v2/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		require.Equal(t, "/v2/plugin/referrers/"+digest, request.URL.Path)
		w.Header().Set("Content-Type", string(types.OCIImageIndex))
		require.NoError(t, json.NewEncoder(w).Encode(index))
	}))
	t.Cleanup(server.Close)
	ref, err := name.NewDigest(server.Listener.Addr().String() + "/plugin@" + digest)
	require.NoError(t, err)
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	client := NewClient(nil, WithKeychain(CredentialKeychain{Store: store}))
	result, err := client.Referrers(t.Context(), ref)
	require.NoError(t, err)
	manifest, err := result.IndexManifest()
	require.NoError(t, err)
	require.Equal(t, index, *manifest)
}

func TestReferrersPreservesCredentialFailure(t *testing.T) {
	ref, err := name.NewDigest("registry.example.com/plugin@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	client := NewClient(nil, WithKeychain(CredentialKeychain{Open: func() (keyring.Keyring, error) {
		return nil, keyring.ErrPasswordRequired
	}}))
	index, err := client.Referrers(t.Context(), ref)
	require.ErrorIs(t, err, keyring.ErrPasswordRequired)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	require.Nil(t, index)
}

func TestReferrersHonorsTransportAndCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	ref, err := name.NewDigest(server.Listener.Addr().String() + "/plugin@sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	client := NewClient(nil, WithKeychain(CredentialKeychain{Store: store}), WithTransport(server.Client().Transport))
	index, err := client.Referrers(t.Context(), ref)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	require.Nil(t, index)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	index, err = client.Referrers(ctx, ref)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, errUtils.ErrOCIRegistry)
	require.Nil(t, index)
}
