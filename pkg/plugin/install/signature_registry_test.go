package install

import (
	"context"
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
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSignedRegistryInstallRevalidatesCachedEvidence(t *testing.T) {
	service, release, _ := installFixture(t)
	authority := signaturetest.New(t)
	service.Verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }
	identity, issuer := "builder@example.com", "https://issuer.example.com"
	service.Config.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "acme", Identity: identity, Issuer: issuer}}
	client, repository, requests := signatureRegistry(t, release)
	service.Registry = client
	raw, err := release.Index.RawManifest()
	require.NoError(t, err)
	bundle := authority.Sign(t, identity, issuer, raw)
	attachSignature(t, client, repository, bundle)
	entry, err := service.Prepare(t.Context(), &Request{Source: "oci://" + repository.Name(), Constraint: "1.0.0"})
	require.NoError(t, err)
	require.Equal(t, "verified", entry.Verification.Signature)
	require.Equal(t, identity, entry.Verification.Identity)
	require.Equal(t, issuer, entry.Verification.Issuer)
	require.Positive(t, requests.Load())

	requests.Store(0)
	service.Config.Plugins.Policy.Offline = true
	request := &Request{Source: entry.Source, Constraint: entry.Constraint, Locked: entry}
	entry.Verification.Identity = "forged@example.com"
	restored, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, identity, restored.Verification.Identity, "the certificate, not the editable lock verdict, determines signer identity")
	require.Equal(t, "forged@example.com", entry.Verification.Identity, "verification must not mutate the caller's lock")
	require.Zero(t, requests.Load())

	// Online operation can repair missing evidence using the exact locked digest.
	cached, err := service.Cache.location(entry.IndexDigest, service.Platform)
	require.NoError(t, err)
	require.NoError(t, os.Remove(filepath.Join(cached, "signatures.json")))
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Zero(t, requests.Load())
	service.Config.Plugins.Policy.Offline = false
	restored, err = service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, identity, restored.Verification.Identity)
	require.Positive(t, requests.Load())

	// Tightened policy must reject old evidence, then accept a newly attached signer.
	service.Config.Plugins.Policy.TrustedPublishers[0].Identity = "replacement@example.com"
	service.Config.Plugins.Policy.Offline = true
	requests.Store(0)
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Zero(t, requests.Load())
	attachSignature(t, client, repository, authority.Sign(t, "replacement@example.com", issuer, raw))
	service.Config.Plugins.Policy.Offline = false
	restored, err = service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "replacement@example.com", restored.Verification.Identity)
	require.Equal(t, entry.IndexDigest, restored.IndexDigest)
}

func TestPublisherConstraintsRequireOneSignerToSatisfyEveryLayer(t *testing.T) {
	service, release, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	authority := signaturetest.New(t)
	service.Verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }
	issuer := "https://issuer.example.com"
	raw, err := release.Index.RawManifest()
	require.NoError(t, err)
	bundles := [][]byte{authority.Sign(t, "alice@example.com", issuer, raw), authority.Sign(t, "bob@example.com", issuer, raw)}
	require.NoError(t, signature.Save(layout, bundles))
	service.Config.Plugins.Policy = schema.PluginPolicy{
		Offline:              true,
		TrustedPublishers:    []schema.PluginPublisher{{Publisher: "acme", Identity: "alice@example.com", Issuer: issuer}},
		PublisherConstraints: [][]schema.PluginPublisher{{{Publisher: "acme", Identity: "bob@example.com", Issuer: issuer}}},
	}
	request := &Request{Source: "oci://ghcr.io/acme/fixture", Constraint: "1.0.0", Layout: layout}
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid, "separate certificates cannot independently satisfy mandatory policy layers")
	cached, err := service.Cache.location(release.Digest, service.Platform)
	require.NoError(t, err)
	_, err = os.Stat(cached)
	require.ErrorIs(t, err, os.ErrNotExist)
	service.Config.Plugins.Policy.PublisherConstraints[0][0].Identity = "*@example.com"
	entry, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "alice@example.com", entry.Verification.Identity)
}

func signatureRegistry(t *testing.T, release *pluginoci.Release) (*oci.Client, name.Repository, *atomic.Int64) {
	t.Helper()
	requests := &atomic.Int64{}
	handler := registry.New(registry.Logger(log.New(io.Discard, "", 0)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	repository, err := name.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/acme/fixture")
	require.NoError(t, err)
	client := oci.NewClient(nil)
	require.NoError(t, client.WriteIndex(t.Context(), repository.Tag("1.0.0"), release.Index))
	requests.Store(0)
	return client, repository, requests
}

func attachSignature(t *testing.T, client *oci.Client, repository name.Repository, raw []byte) {
	t.Helper()
	subject, err := client.Head(t.Context(), repository.Tag("1.0.0"))
	require.NoError(t, err)
	image, err := mutate.AppendLayers(empty.Image, static.NewLayer(raw, types.MediaType("application/json")))
	require.NoError(t, err)
	image = mutate.MediaType(image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.MediaType("application/vnd.dev.sigstore.bundle.v0.3+json"))
	image = mutate.Subject(image, *subject).(v1.Image)
	digest, err := image.Digest()
	require.NoError(t, err)
	require.NoError(t, client.WriteImage(t.Context(), repository.Digest(digest.String()), image))
}

func TestRegistryWithNoSignatureCannotPopulateCache(t *testing.T) {
	service, release, _ := installFixture(t)
	client, repository, requests := signatureRegistry(t, release)
	service.Registry = client
	service.Config.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
	_, err := service.Prepare(t.Context(), &Request{Source: "oci://" + repository.Name(), Constraint: "1.0.0"})
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.Positive(t, requests.Load(), "online admission must check the registry for evidence")
	directory, err := service.Cache.location(release.Digest, service.Platform)
	require.NoError(t, err)
	require.NoDirExists(t, directory, "unsigned releases must not leave executable cache entries")
}
