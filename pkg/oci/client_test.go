package oci_test

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/validate"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/keyring"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestIndexRoundTripWithIsolatedRegistryCredentials(t *testing.T) {
	ctx := t.Context()
	source, sourceRef, closeSource := credentialRegistry(t, "source-only")
	destination, destinationRef, _ := credentialRegistry(t, "destination-only")
	index := testIndex(t)
	want, err := index.Digest()
	require.NoError(t, err)
	require.NoError(t, source.WriteIndex(ctx, sourceRef, index))
	// Passing a source client's credentials to the destination must fail.
	require.Error(t, source.WriteIndex(ctx, destinationRef, index))
	directory := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, oci.ExportIndex(ctx, source, sourceRef, directory))
	closeSource()
	require.NoError(t, oci.ImportIndex(ctx, destination, directory, destinationRef))
	resolved, err := destination.Get(ctx, destinationRef)
	require.NoError(t, err)
	require.Equal(t, want, resolved.Digest)
	result, err := resolved.ImageIndex()
	require.NoError(t, err)
	require.NoError(t, validate.Index(result))
	manifest, err := result.IndexManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Manifests, 2)
	require.Equal(t, "amd64", manifest.Manifests[0].Platform.Architecture)
	require.Equal(t, "arm64", manifest.Manifests[1].Platform.Architecture)
	tags, err := destination.Tags(ctx, destinationRef.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"1.0.0"}, tags)
	head, err := destination.Head(ctx, destinationRef)
	require.NoError(t, err)
	require.Equal(t, want, head.Digest)
	require.NoError(t, destination.Delete(ctx, destinationRef.Context().Digest(want.String())))
	_, err = destination.Get(ctx, destinationRef.Context().Digest(want.String()))
	require.Error(t, err)
}

func credentialRegistry(t *testing.T, password string) (*oci.Client, name.Tag, func()) {
	t.Helper()
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, supplied, _ := r.BasicAuth()
		if user != "publisher" || supplied != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	chain := oci.CredentialKeychain{Store: store}
	require.NoError(t, chain.Login(host, &authn.AuthConfig{Username: "publisher", Password: password}))
	ref, err := name.NewTag(host + "/acme/plugin:1.0.0")
	require.NoError(t, err)
	return oci.NewClient(nil, oci.WithKeychain(chain)), ref, server.Close
}

func testIndex(t *testing.T) v1.ImageIndex {
	t.Helper()
	var additions []mutate.IndexAddendum
	for _, arch := range []string{"amd64", "arm64"} {
		img, err := random.Image(128, 1)
		require.NoError(t, err)
		config, err := img.ConfigFile()
		require.NoError(t, err)
		config.OS, config.Architecture = "linux", arch
		img, err = mutate.ConfigFile(img, config)
		require.NoError(t, err)
		additions = append(additions, mutate.IndexAddendum{Add: img, Descriptor: v1.Descriptor{Platform: &v1.Platform{OS: "linux", Architecture: arch}}})
	}
	return mutate.AppendManifests(empty.Index, additions...)
}

func TestLayoutRejectsChangedRootAndMissingBlobs(t *testing.T) {
	directory := t.TempDir()
	index := testIndex(t)
	require.NoError(t, oci.SaveIndex(directory, index))
	want, err := index.Digest()
	require.NoError(t, err)
	_, err = oci.LoadIndex(directory, "sha256:incorrect")
	require.Error(t, err)
	rootBlob := filepath.Join(directory, "blobs", want.Algorithm, want.Hex)
	require.NoError(t, os.WriteFile(rootBlob, []byte(`{"schemaVersion":2,"manifests":[]}`), 0o600))
	_, err = oci.LoadIndex(directory, want.String())
	require.Error(t, err)
	require.NoError(t, os.Remove(rootBlob))
	_, err = oci.LoadIndex(directory, want.String())
	require.Error(t, err)
}

func TestClientCancellation(t *testing.T) {
	client, ref, _ := credentialRegistry(t, "test-only")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := client.Get(ctx, ref)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, client.WriteIndex(ctx, ref, testIndex(t)), context.Canceled)
}

func TestCredentialScopeAndGHCRFallback(t *testing.T) {
	store, err := keyring.New(keyring.Config{Type: keyring.TypeMemory})
	require.NoError(t, err)
	chain := oci.CredentialKeychain{Store: store}
	require.NoError(t, chain.Login("ghcr.io", &authn.AuthConfig{Username: "explicit", Password: "fixture"}))
	repo, err := name.NewRepository("ghcr.io/acme/plugin")
	require.NoError(t, err)
	config := &schema.AtmosConfiguration{}
	config.Settings.GithubUsername = "environment"
	config.Settings.GithubToken = "fixture-env"
	auth, _, err := oci.ResolveAuthentication(config, chain, repo)
	require.NoError(t, err)
	resolved, err := auth.Authorization()
	require.NoError(t, err)
	require.Equal(t, "explicit", resolved.Username)
	require.NoError(t, chain.Logout("ghcr.io"))
	auth, _, err = oci.ResolveAuthentication(config, chain, repo)
	require.NoError(t, err)
	resolved, err = auth.Authorization()
	require.NoError(t, err)
	require.Equal(t, "environment", resolved.Username)
	other, err := name.NewRepository("example.com/acme/plugin")
	require.NoError(t, err)
	auth, _, err = oci.ResolveAuthentication(config, chain, other)
	require.NoError(t, err)
	require.Equal(t, authn.Anonymous, auth)
}

func TestSaveSelectionKeepsMetadataAndOnlySelectedLayers(t *testing.T) {
	index := testIndex(t)
	manifest, err := index.IndexManifest()
	require.NoError(t, err)
	require.Len(t, manifest.Manifests, 2)
	directory := t.TempDir()
	require.NoError(t, oci.SaveSelection(directory, index, []v1.Hash{manifest.Manifests[0].Digest}))
	rootDigest, err := index.Digest()
	require.NoError(t, err)
	restored, err := oci.LoadIndex(directory, rootDigest.String())
	require.NoError(t, err)
	raw, err := restored.RawManifest()
	require.NoError(t, err)
	original, err := index.RawManifest()
	require.NoError(t, err)
	require.Equal(t, original, raw, "the release index must remain byte-identical")
	for position, descriptor := range manifest.Manifests {
		image, err := restored.Image(descriptor.Digest)
		require.NoError(t, err)
		config, err := image.ConfigFile()
		require.NoError(t, err)
		require.Equal(t, descriptor.Platform.Architecture, config.Architecture)
		layers, err := image.Layers()
		require.NoError(t, err)
		require.Len(t, layers, 1)
		reader, err := layers[0].Compressed()
		if position != 0 {
			require.ErrorIs(t, err, os.ErrNotExist, "unselected platform layers must not be downloaded")
			continue
		}
		require.NoError(t, err)
		content, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		require.NotEmpty(t, content)
		require.NoError(t, validate.Image(image))
	}
}

func TestSaveSelectionRejectsNestedIndexesAndBlockedDestination(t *testing.T) {
	nested := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: testIndex(t)})
	require.ErrorIs(t, oci.SaveSelection(t.TempDir(), nested, nil), errUtils.ErrOCILayout)
	blocked := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocked, []byte("keep"), 0o600))
	require.ErrorIs(t, oci.SaveSelection(blocked, testIndex(t), nil), errUtils.ErrOCILayout)
	require.ErrorIs(t, oci.SaveIndex(blocked, testIndex(t)), errUtils.ErrOCILayout)
	actual, err := os.ReadFile(blocked)
	require.NoError(t, err)
	require.Equal(t, "keep", string(actual))
}

func TestSaveSelectionRejectsMissingAndCorruptPlatformBlobs(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"missing manifest", "changed manifest", "missing config", "changed config", "missing selected layer"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			original := testIndex(t)
			source := t.TempDir()
			require.NoError(t, oci.SaveIndex(source, original))
			index, err := oci.LoadIndex(source, "")
			require.NoError(t, err)
			root, err := original.IndexManifest()
			require.NoError(t, err)
			descriptor := root.Manifests[0]
			image, err := original.Image(descriptor.Digest)
			require.NoError(t, err)
			manifest, err := image.Manifest()
			require.NoError(t, err)
			digest := descriptor.Digest
			switch failure {
			case "missing config", "changed config":
				digest = manifest.Config.Digest
			case "missing selected layer":
				digest = manifest.Layers[0].Digest
			}
			blob := filepath.Join(source, "blobs", digest.Algorithm, digest.Hex)
			if strings.HasPrefix(failure, "missing") {
				require.NoError(t, os.Remove(blob))
			} else {
				require.NoError(t, os.WriteFile(blob, []byte(`{"tampered":true}`), 0o600))
			}
			destination := t.TempDir()
			err = oci.SaveSelection(destination, index, []v1.Hash{descriptor.Digest})
			require.Error(t, err)
			if strings.HasPrefix(failure, "missing") {
				require.ErrorIs(t, err, os.ErrNotExist)
			} else {
				require.ErrorIs(t, err, errUtils.ErrOCIIntegrity)
			}
			_, err = oci.LoadIndex(destination, "")
			require.ErrorIs(t, err, errUtils.ErrOCILayout, "an incomplete selection must never publish a usable root index")
		})
	}
}

func TestLayoutImportRejectsInvalidRootsBeforeRegistryAccess(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"missing layout", "malformed root", "empty root", "image root"} {
		t.Run(failure, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			if failure != "missing layout" {
				path, err := layout.Write(directory, empty.Index)
				require.NoError(t, err)
				if failure == "malformed root" {
					require.NoError(t, os.WriteFile(filepath.Join(directory, "index.json"), []byte("invalid JSON"), 0o600))
				}
				if failure == "image root" {
					require.NoError(t, path.AppendImage(empty.Image))
				}
			}
			// A nil registry would panic if import reached an upload before validating the layout.
			require.ErrorIs(t, oci.ImportIndex(t.Context(), nil, directory, nil), errUtils.ErrOCILayout)
		})
	}
}

func TestExportIndexRejectsRegistryFailureAndOrdinaryImage(t *testing.T) {
	t.Parallel()
	client, ref, _ := credentialRegistry(t, "export-test")
	for _, published := range []bool{false, true} {
		if published {
			require.NoError(t, client.WriteImage(t.Context(), ref, empty.Image))
		}
		destination := filepath.Join(t.TempDir(), "export")
		err := oci.ExportIndex(t.Context(), client, ref, destination)
		require.Error(t, err)
		if published {
			require.ErrorIs(t, err, errUtils.ErrOCILayout)
		} else {
			require.True(t, oci.IsNotFound(err))
		}
		require.NoDirExists(t, destination, "failed export must not publish a local layout")
	}
}
