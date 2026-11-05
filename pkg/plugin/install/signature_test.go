package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/oci/signature/signaturetest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSignedInstallOfflineReuseAndPolicyChanges(t *testing.T) {
	service, release, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	authority := signaturetest.New(t)
	identity, issuer := "https://github.com/acme/oidc/release", "https://token.actions.githubusercontent.com"
	rawIndex, err := release.Index.RawManifest()
	require.NoError(t, err)
	evidence := authority.Sign(t, identity, issuer, rawIndex)
	require.NoError(t, signature.Save(layout, [][]byte{evidence}))
	service.Config.Plugins.Policy = schema.PluginPolicy{Offline: true, RequireSignature: true, TrustedPublishers: []schema.PluginPublisher{{Publisher: "acme", Identity: identity, Issuer: issuer}}}
	service.Verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) { return authority, nil }
	service.Registry = NewMockRegistry(gomock.NewController(t)) // Offline paths must never consult registry.
	request := &Request{Source: "oci://ghcr.io/acme/fixture", Constraint: "1.0.0", Layout: layout}
	entry, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "verified", entry.Verification.Signature)
	require.Equal(t, identity, entry.Verification.Identity)
	request.Layout = ""
	request.Locked = entry
	restored, err := service.Prepare(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, entry, restored)
	// A forged lock verdict cannot bypass fresh verification against current policy.
	service.Config.Plugins.Policy.TrustedPublishers[0].Identity = "https://github.com/other/*"
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	service.Config.Plugins.Policy.TrustedPublishers[0].Identity = identity
	service.Config.Plugins.Policy.PublisherConstraints = [][]schema.PluginPublisher{{{Publisher: "acme", Identity: "https://github.com/other/*", Issuer: issuer}}}
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	service.Config.Plugins.Policy.PublisherConstraints = nil
	cached, err := service.Cache.location(entry.IndexDigest, service.Platform)
	require.NoError(t, err)
	require.NoError(t, signature.Save(cached, [][]byte{[]byte(`{}`)}))
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	require.NoError(t, os.Remove(filepath.Join(cached, "signatures.json")))
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
}

func TestUnsignedAndWrongPublisherCannotPopulateCache(t *testing.T) {
	service, release, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	service.Config.Plugins.Policy = schema.PluginPolicy{Offline: true, TrustedPublishers: []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}}
	request := &Request{Source: "oci://ghcr.io/acme/fixture", Constraint: "*", Layout: layout}
	_, err := service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	cached, err := service.Cache.location(release.Digest, service.Platform)
	require.NoError(t, err)
	_, err = os.Stat(cached)
	require.ErrorIs(t, err, os.ErrNotExist)
	service.Config.Plugins.Policy.TrustedPublishers[0].Publisher = "other"
	_, err = service.Prepare(t.Context(), request)
	require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
	_, err = os.Stat(cached)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestSignatureVerificationStopsWhenTrustRootLookupIsCanceled(t *testing.T) {
	service, release, _ := installFixture(t)
	authority := signaturetest.New(t)
	raw, err := release.Index.RawManifest()
	require.NoError(t, err)
	identity := signature.Identity{Identity: "builder@example.com", Issuer: "https://issuer.example.com"}
	bundle := authority.Sign(t, identity.Identity, identity.Issuer, raw)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	lookups := 0
	service.Verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) {
		lookups++
		cancel()
		return nil, context.Canceled
	}
	_, err = service.verifyBundles(ctx, release.Digest, [][]signature.Identity{{identity}}, [][]byte{bundle, bundle})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, lookups, "cancellation must not continue trying other signatures")
}

func TestVerifyInstalledRejectsMissingPayloadBeforeTrustRootLookup(t *testing.T) {
	service, release, _ := installFixture(t)
	service.Config.Plugins.Policy.RequireSignature = true
	service.Verifier.LoadRoot = func(context.Context, bool) (root.TrustedMaterial, error) {
		t.Fatal("missing executable bytes must fail before loading trust material")
		return nil, nil
	}
	_, err := service.VerifyInstalled(t.Context(), lockedEntry(release))
	require.ErrorIs(t, err, errUtils.ErrPluginNotInstalled)
}

func TestRequiredSignatureFailsClosedWithoutPublisherTrustOrDiscovery(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(map[bool]string{false: "no publisher trust", true: "transport cannot discover signatures"}[trusted], func(t *testing.T) {
			service, release, _ := installFixture(t)
			_, err := service.Cache.Store(t.Context(), release, CurrentPlatform(), service.Descriptors)
			require.NoError(t, err)
			service.Config.Plugins.Policy.RequireSignature = true
			if trusted {
				service.Config.Plugins.Policy.TrustedPublishers = []schema.PluginPublisher{{Publisher: "acme", Identity: "builder@example.com", Issuer: "https://issuer.example.com"}}
			}
			// This registry implements ordinary OCI transport, without referrer discovery.
			service.Registry = NewMockRegistry(gomock.NewController(t))
			entry := lockedEntry(release)
			entry.Source = "oci://registry.example.com/acme/fixture"
			verification, err := service.VerifyInstalled(t.Context(), entry)
			require.ErrorIs(t, err, errUtils.ErrPluginSignatureInvalid)
			require.Empty(t, verification.Signature, "available executable bytes are not proof of a trusted publisher")
		})
	}
}
