package oci_test

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
)

// emptyManifest pushes a minimal artifact manifest under reference and returns its descriptor.
func pushManifest(t *testing.T, store oci.RepositoryStore, reference, title string) ocispec.Descriptor {
	t.Helper()
	config := oci.Descriptor(ocispec.MediaTypeEmptyJSON, []byte("{}"))
	exists, err := store.Exists(t.Context(), config)
	require.NoError(t, err)
	if !exists {
		require.NoError(t, store.Push(t.Context(), config, bytes.NewReader([]byte("{}"))))
	}
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.example","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"` + config.Digest.String() + `","size":2},"layers":[],"annotations":{"title":"` + title + `"}}`)
	descriptor := oci.Descriptor(ocispec.MediaTypeImageManifest, manifest)
	require.NoError(t, store.PushReference(t.Context(), descriptor, bytes.NewReader(manifest), reference))
	return descriptor
}

func TestClientRepositoriesListsTheRegistryCatalogWithCredentials(t *testing.T) {
	server, client := authenticatedRegistry(t, "secret")
	for _, path := range []string{"acme/one", "acme/two/deep", "other/three"} {
		store, err := client.Repository(t.Context(), oci.Repository{Registry: server.Host, Path: path})
		require.NoError(t, err)
		pushManifest(t, store, "v1", path)
	}
	var catalog oci.RegistryCatalog = client
	repositories, err := catalog.Repositories(t.Context(), server.Host)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"acme/one", "acme/two/deep", "other/three"}, repositories)

	anonymous := oci.NewClient(nil, oci.WithKeychain(authn.NewMultiKeychain()))
	_, err = anonymous.Repositories(t.Context(), server.Host)
	require.Error(t, err, "a missing credential is an error, never an empty catalog")

	_, err = client.Repositories(t.Context(), "not a registry!")
	require.Error(t, err)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = client.Repositories(canceled, server.Host)
	require.Error(t, err)
}

func TestLayoutUntagRemovesOnlyTheReference(t *testing.T) {
	directory := t.TempDir()
	layout, err := oci.OpenLayout(t.Context(), directory)
	require.NoError(t, err)
	body := []byte("blob")
	blob := oci.Descriptor("application/octet-stream", body)
	require.NoError(t, layout.Push(t.Context(), blob, bytes.NewReader(body)))
	config := oci.Descriptor(ocispec.MediaTypeEmptyJSON, []byte("{}"))
	require.NoError(t, layout.Push(t.Context(), config, bytes.NewReader([]byte("{}"))))
	manifest := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"` + config.Digest.String() + `","size":2},"layers":[{"mediaType":"application/octet-stream","digest":"` + blob.Digest.String() + `","size":4}]}`)
	root := oci.Descriptor(ocispec.MediaTypeImageManifest, manifest)
	require.NoError(t, layout.Push(t.Context(), root, bytes.NewReader(manifest)))
	require.NoError(t, layout.Tag(t.Context(), root, "a:1"))
	require.NoError(t, layout.Tag(t.Context(), root, "a:2"))
	assert.Equal(t, []string{"a:1", "a:2"}, layout.References())

	require.NoError(t, layout.Untag(t.Context(), "a:1"))
	assert.Equal(t, []string{"a:2"}, layout.References())
	_, err = layout.Resolve(t.Context(), "a:1")
	require.ErrorIs(t, err, errdef.ErrNotFound)
	resolved, err := layout.Resolve(t.Context(), "a:2")
	require.NoError(t, err)
	assert.Equal(t, root.Digest, resolved.Digest)
	exists, err := layout.Exists(t.Context(), blob)
	require.NoError(t, err)
	assert.True(t, exists, "blobs are never deleted: they may be shared")

	reopened, err := oci.OpenLayout(t.Context(), directory)
	require.NoError(t, err)
	assert.Equal(t, []string{"a:2"}, reopened.References(), "the removal was persisted")
	require.Error(t, layout.Untag(t.Context(), "a:1"), "an absent reference cannot be untagged")
	require.Error(t, layout.Untag(t.Context(), root.Digest.String()), "a digest is not a tag")
	require.Error(t, layout.Untag(t.Context(), ""))
}

func TestIsAlreadyExists(t *testing.T) {
	assert.True(t, oci.IsAlreadyExists(errdef.ErrAlreadyExists))
	assert.True(t, oci.IsAlreadyExists(errors.Join(errors.New("push"), errdef.ErrAlreadyExists)))
	assert.False(t, oci.IsAlreadyExists(errdef.ErrNotFound))
	assert.False(t, oci.IsAlreadyExists(nil))
}

func TestTestRegistryOptions(t *testing.T) {
	t.Run("tags follow a manifest delete like a production registry", func(t *testing.T) {
		server := ocitest.NewServerWithOptions(t, nil, ocitest.WithTagsFollowManifestDelete())
		client := oci.NewClient(nil, oci.WithKeychain(authn.NewMultiKeychain()))
		store, err := client.Repository(t.Context(), oci.Repository{Registry: server.Host, Path: "acme/app"})
		require.NoError(t, err)
		first := pushManifest(t, store, "v1", "first")
		require.NoError(t, store.Tag(t.Context(), first, "stable"))
		kept := pushManifest(t, store, "v2", "second")

		require.NoError(t, store.Delete(t.Context(), first))
		for _, tag := range []string{"v1", "stable"} {
			_, err = store.Resolve(t.Context(), tag)
			require.ErrorIs(t, err, errdef.ErrNotFound, tag)
		}
		resolved, err := store.Resolve(t.Context(), "v2")
		require.NoError(t, err)
		assert.Equal(t, kept.Digest, resolved.Digest)
	})
	t.Run("the bare registry keeps tags of a deleted manifest", func(t *testing.T) {
		server := ocitest.NewServer(t, nil)
		client := oci.NewClient(nil, oci.WithKeychain(authn.NewMultiKeychain()))
		store, err := client.Repository(t.Context(), oci.Repository{Registry: server.Host, Path: "acme/app"})
		require.NoError(t, err)
		first := pushManifest(t, store, "v1", "first")
		require.NoError(t, store.Delete(t.Context(), first))
		_, err = store.Resolve(t.Context(), "v1")
		require.NoError(t, err)
	})
	t.Run("referrers fall back to the tag schema", func(t *testing.T) {
		server := ocitest.NewServerWithOptions(t, nil, ocitest.WithoutReferrersAPI())
		client := oci.NewClient(nil, oci.WithKeychain(authn.NewMultiKeychain()))
		store, err := client.Repository(t.Context(), oci.Repository{Registry: server.Host, Path: "acme/app"})
		require.NoError(t, err)
		subject := pushManifest(t, store, "v1", "subject")
		attachment := []byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","artifactType":"application/vnd.example.sig","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"` + digest.FromString("{}").String() + `","size":2},"layers":[],"subject":{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"` + subject.Digest.String() + `","size":` + itoa(subject.Size) + `}}`)
		descriptor := oci.Descriptor(ocispec.MediaTypeImageManifest, attachment)
		require.NoError(t, store.Push(t.Context(), descriptor, bytes.NewReader(attachment)))
		found, err := store.Referrers(t.Context(), subject, "")
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, descriptor.Digest, found[0].Digest)
	})
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
