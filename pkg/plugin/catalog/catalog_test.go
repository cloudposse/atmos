package catalog_test

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
)

func TestCatalogPublicationConflictAndUnlisting(t *testing.T) {
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	ref, err := name.NewTag(server.Listener.Addr().String() + "/acme/catalog:stable")
	require.NoError(t, err)
	client := oci.NewClient(nil)
	file := catalog.New("acme")
	file.Spec.Entries = []catalog.Entry{entry()}
	require.NoError(t, catalog.Publish(t.Context(), client, ref.Name(), file, ""))
	first, digest, err := catalog.Read(t.Context(), client, ref.Name())
	require.NoError(t, err)
	require.Equal(t, 1, first.Metadata.Revision)
	require.Len(t, catalog.Search(first, catalog.Filter{Capability: "auth.provider", Query: "OIDC"}), 1)
	require.Empty(t, catalog.Search(first, catalog.Filter{Runtime: "starlark"}))
	concurrent, staleDigest, err := catalog.Read(t.Context(), client, ref.Name())
	require.NoError(t, err)
	require.NoError(t, catalog.Remove(first, "acme/auth", "1.0.0"))
	require.NoError(t, catalog.Publish(t.Context(), client, ref.Name(), first, digest))
	require.ErrorIs(t, catalog.Publish(t.Context(), client, ref.Name(), concurrent, staleDigest), errUtils.ErrPluginCatalogConflict)
	after, _, err := catalog.Read(t.Context(), client, ref.Name())
	require.NoError(t, err)
	require.Equal(t, 2, after.Metadata.Revision)
	require.Len(t, after.Spec.Entries[0].Versions, 1)
}

func TestSelectExcludesYankedAndIncompatiblePlatforms(t *testing.T) {
	value := entry()
	selected, err := catalog.Select(&value, "^1.0.0", "linux/amd64")
	require.NoError(t, err)
	require.Equal(t, "1.0.0", selected.Version)
	value.Versions[1].Yanked = nil
	selected, err = catalog.Select(&value, "^1.0.0", "linux/amd64")
	require.NoError(t, err)
	require.Equal(t, "1.1.0", selected.Version)
	_, err = catalog.Select(&value, "^1.0.0", "windows/arm64")
	require.ErrorIs(t, err, errUtils.ErrPluginVersionNotFound)
}

func entry() catalog.Entry {
	return catalog.Entry{Name: "acme/auth", Description: "OIDC authentication", Publisher: "acme", Capabilities: []string{"auth.provider"}, Runtime: "native", Source: "oci://ghcr.io/acme/auth", Versions: []catalog.Version{
		{Version: "1.0.0", Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Platforms: []string{"linux/amd64"}, Compatibility: manifest.Compatibility{Protocol: ">=1 <2"}},
		{Version: "1.1.0", Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Platforms: []string{"linux/amd64"}, Compatibility: manifest.Compatibility{Protocol: ">=1 <2"}, Yanked: &catalog.Yanked{Reason: "regression"}},
	}}
}

func TestCatalogRejectsInvalidArtifactsAndPublication(t *testing.T) {
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	client := oci.NewClient(nil)
	ref, err := name.NewTag(server.Listener.Addr().String() + "/acme/catalog:stable")
	require.NoError(t, err)
	t.Run("invalid references", func(t *testing.T) {
		file, digest, err := catalog.Read(t.Context(), client, "invalid reference")
		require.Error(t, err)
		require.Nil(t, file)
		require.Empty(t, digest)
		require.Error(t, catalog.Publish(t.Context(), client, "invalid reference", catalog.New("acme"), ""))
	})
	t.Run("deleted revision", func(t *testing.T) {
		file := catalog.New("acme")
		file.Metadata.Revision = 1
		require.ErrorIs(t, catalog.Publish(t.Context(), client, ref.Name(), file, "previous-digest"), errUtils.ErrPluginCatalogConflict)
		_, _, err := catalog.Read(t.Context(), client, ref.Name())
		require.True(t, oci.IsNotFound(err), "conflict must not recreate a deleted catalog")
	})
	t.Run("invalid catalog", func(t *testing.T) {
		file := catalog.New("acme")
		invalid := entry()
		invalid.Name = ""
		file.Spec.Entries = []catalog.Entry{invalid}
		require.ErrorIs(t, catalog.Publish(t.Context(), client, ref.Name(), file, ""), errUtils.ErrPluginInvalidConfig)
	})
	t.Run("oversized catalog", func(t *testing.T) {
		file := catalog.New("acme")
		oversized := entry()
		oversized.Description = strings.Repeat("x", 8<<20)
		file.Spec.Entries = []catalog.Entry{oversized}
		require.ErrorIs(t, catalog.Publish(t.Context(), client, ref.Name(), file, ""), errUtils.ErrPluginPayloadTooLarge)
	})
	for _, tc := range []struct {
		name          string
		config, layer types.MediaType
		payload       string
		want          error
	}{
		{"other artifact", types.OCIConfigJSON, types.OCILayer, "{}", errUtils.ErrPluginInvalidConfig},
		{"malformed catalog", "application/vnd.atmos.plugin.catalog.config.v1+json", "application/vnd.atmos.plugin.catalog.v1+json", "{", errUtils.ErrManifestParse},
		{"oversized layer", "application/vnd.atmos.plugin.catalog.config.v1+json", "application/vnd.atmos.plugin.catalog.v1+json", strings.Repeat("x", 8<<20+1), errUtils.ErrOCIIntegrity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			img, err := mutate.AppendLayers(empty.Image, static.NewLayer([]byte(tc.payload), tc.layer))
			require.NoError(t, err)
			img = mutate.ConfigMediaType(mutate.MediaType(img, types.OCIManifestSchema1), tc.config)
			require.NoError(t, client.WriteImage(t.Context(), ref, img))
			file, _, err := catalog.Read(t.Context(), client, ref.Name())
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, file)
			require.ErrorIs(t, catalog.Publish(t.Context(), client, ref.Name(), catalog.New("acme"), ""), tc.want, "unreadable remote state must not be overwritten")
		})
	}
}

func TestMissingCatalogBlobCannotReplaceHistory(t *testing.T) {
	var missingBlob atomic.Bool
	var mutations atomic.Int64
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			mutations.Add(1)
		}
		if missingBlob.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/acme/catalog/blobs/") {
			http.Error(w, "catalog blob missing", http.StatusNotFound)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client := oci.NewClient(nil)
	reference := server.Listener.Addr().String() + "/acme/catalog:stable"
	original := catalog.New("acme")
	original.Spec.Entries = []catalog.Entry{entry()}
	// A genuinely absent manifest still permits the first catalog publication.
	require.NoError(t, catalog.Publish(t.Context(), client, reference, original, ""))
	original, digest, err := catalog.Read(t.Context(), client, reference)
	require.NoError(t, err)
	before := mutations.Load()
	missingBlob.Store(true)
	file, observed, err := catalog.Read(t.Context(), client, reference)
	require.Nil(t, file)
	require.True(t, oci.IsNotFound(err), "retain the underlying missing-blob cause")
	require.Equal(t, digest, observed, "a present manifest cannot be mistaken for a missing catalog")

	for _, expected := range []string{"", digest} {
		err = catalog.Publish(t.Context(), client, reference, catalog.New("replacement"), expected)
		var missing *transport.Error
		require.ErrorAs(t, err, &missing, "publication must fail on the unreadable catalog rather than replacing it")
		require.Equal(t, http.StatusNotFound, missing.StatusCode)
	}
	selected := []catalog.ReleaseSelection{{Name: "auth", Repository: "oci://" + server.Listener.Addr().String() + "/acme/auth", Version: "1.0.0"}}
	err = catalog.NewSyncer(client, nil).Sync(t.Context(), reference, selected, catalog.WithSkipMissing())
	var missing *transport.Error
	require.ErrorAs(t, err, &missing, "skip-missing applies to release tags, never catalog blobs")
	require.Equal(t, http.StatusNotFound, missing.StatusCode)
	require.Equal(t, before, mutations.Load(), "neither path may write through the unreadable history")

	missingBlob.Store(false)
	after, afterDigest, err := catalog.Read(t.Context(), client, reference)
	require.NoError(t, err)
	require.Equal(t, original, after, "historical versions, revision, and yank metadata remain intact")
	require.Equal(t, digest, afterDigest)
}
