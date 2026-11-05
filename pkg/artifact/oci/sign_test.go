//nolint:gocritic // OCI descriptors and the pinned reference are passed by value, mirroring the ORAS content contracts and the fixed artifact.Repository signatures.
package oci

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	sigroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/sigstore"
	"github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

const (
	trustedIdentity = "https://github.com/acme/images/.github/workflows/release.yml@refs/heads/main"
	trustedIssuer   = "https://token.actions.githubusercontent.com"
)

// keylessBlob signs with a certificate issued by the isolated test authority.
type keylessBlob struct {
	t        *testing.T
	auth     *signaturetest.Authority
	identity string
	issuer   string
}

func (k keylessBlob) Sign(_ context.Context, data []byte) ([]byte, error) {
	return k.auth.Sign(k.t, k.identity, k.issuer, data), nil
}

// keyBlob signs with a bare public key and remembers it so the verifier can be told to trust it.
type keyBlob struct {
	t    *testing.T
	auth *signaturetest.Authority
	mu   sync.Mutex
	last signaturetest.KeySignature
}

func (k *keyBlob) Sign(_ context.Context, data []byte) ([]byte, error) {
	signed := k.auth.SignWithKey(k.t, data)
	k.mu.Lock()
	k.last = signed
	k.mu.Unlock()
	return signed.Bundle, nil
}

func (k *keyBlob) publicKey() crypto.PublicKey {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.last.PublicKey
}

// trust is one isolated Sigstore trust domain plus the signers and verifiers built on it.
type trust struct {
	t        *testing.T
	auth     *signaturetest.Authority
	keyless  *sigstore.Signer
	keyed    *sigstore.Signer
	keyBlob  *keyBlob
	identity string
}

func newTrust(t *testing.T) *trust {
	t.Helper()
	auth := signaturetest.New(t)
	blob := &keyBlob{t: t, auth: auth}
	return &trust{
		t: t, auth: auth, keyBlob: blob, identity: trustedIdentity,
		keyless: sigstore.NewSigner(keylessBlob{t: t, auth: auth, identity: trustedIdentity, issuer: trustedIssuer}),
		keyed:   sigstore.NewSigner(blob),
	}
}

func (tr *trust) loader(material sigroot.TrustedMaterial) sigstore.VerifierOption {
	return sigstore.WithRootLoader(func(context.Context, bool) (sigroot.TrustedMaterial, error) { return material, nil })
}

// verifier trusts the authority's roots and, optionally, named public keys.
func (tr *trust) verifier(opts ...sigstore.VerifierOption) *sigstore.Verifier {
	return sigstore.NewVerifier(append([]sigstore.VerifierOption{tr.loader(tr.auth)}, opts...)...)
}

func identityPolicy(identity, issuer string) artifact.VerificationPolicy {
	return artifact.VerificationPolicy{
		RequireSignature: true,
		TrustSets:        []artifact.TrustSet{{Identities: []artifact.TrustedIdentity{{Identity: identity, Issuer: issuer}}}},
	}
}

func keyPolicy(keyName string) artifact.VerificationPolicy {
	return artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{{Keys: []string{keyName}}}}
}

// signedEnv publishes one package to a repository built with the verifier.
type signedEnv struct {
	repo    *Repository
	result  *artifact.PublishResult
	record  artifact.SignatureSubject
	rootSub artifact.SignatureSubject
}

func newSignedEnv(t *testing.T, repo *Repository) *signedEnv {
	t.Helper()
	result := publish(t, repo, publishSpec{content: "signed"})
	pinned := mustResolveRef(t, repo, artifact.Ref{Name: appName, Version: appVersion})
	_, loaded, err := repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	subjects := subjectsOf(loaded)
	return &signedEnv{repo: repo, result: result, record: subjects[0], rootSub: subjects[1]}
}

// withVerifier runs a test on a registry and a layout repository that use the verifier.
func withVerifier(t *testing.T, verifier artifact.Verifier, run func(t *testing.T, env *signedEnv)) {
	t.Helper()
	t.Run("registry", func(t *testing.T) {
		run(t, newSignedEnv(t, newRegistryRepo(t, newRegistry(t, nil), WithVerifier(verifier))))
	})
	t.Run("layout", func(t *testing.T) {
		run(t, newSignedEnv(t, newLayoutRepo(t, WithVerifier(verifier))))
	})
}

func (e *signedEnv) sign(t *testing.T, subject artifact.SignatureSubject, signer artifact.Signer) []artifact.SignatureEvidence {
	t.Helper()
	evidence, err := e.repo.Sign(context.Background(), subject, signer, artifact.SignOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, evidence)
	return evidence
}

func (e *signedEnv) verify(subject artifact.SignatureSubject, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	return e.repo.Verify(context.Background(), subject, policy)
}

func (e *signedEnv) requireVerified(t *testing.T, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) *artifact.VerificationResult {
	t.Helper()
	result, err := e.verify(subject, policy)
	require.NoError(t, err)
	require.Equal(t, artifact.VerificationVerified, result.Status)
	return result
}

func (e *signedEnv) requireRejected(t *testing.T, subject artifact.SignatureSubject, policy artifact.VerificationPolicy) *artifact.VerificationResult {
	t.Helper()
	result, err := e.verify(subject, policy)
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
	require.NotNil(t, result)
	require.NotEqual(t, artifact.VerificationVerified, result.Status)
	return result
}

func TestKeylessRecordSignatureIsBoundToTheConfigDescriptor(t *testing.T) {
	tr := newTrust(t)
	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		policy := identityPolicy(trustedIdentity, trustedIssuer)
		env.requireRejected(t, env.record, policy) // unsigned
		env.sign(t, env.record, tr.keyless)

		result := env.requireVerified(t, env.record, policy)
		assert.Equal(t, []artifact.SignerClaim{{Identity: trustedIdentity, Issuer: trustedIssuer}}, result.Claims)
		assert.NotEmpty(t, result.EvidenceDigests)
		assert.NotEmpty(t, result.PolicyFingerprint)
		assert.Equal(t, env.result.Pinned.RecordDigest, env.record.Digest, "the signed Record digest is the config descriptor digest")

		// A signed Record is not a signed native envelope.
		env.requireRejected(t, env.rootSub, policy)
		env.sign(t, env.rootSub, tr.keyless)
		env.requireVerified(t, env.rootSub, policy)

		pinned := mustResolveRef(t, env.repo, artifact.Ref{Name: appName, Version: appVersion})
		pin, err := env.repo.Pin(context.Background(), pinned.Ref)
		require.NoError(t, err)
		assert.Len(t, pin.Evidence, 2, "pins persist the evidence descriptors, not trust")
		inspection, err := env.repo.Inspect(context.Background(), pinned)
		require.NoError(t, err)
		assert.Len(t, inspection.Evidence, 2)
	})
}

func TestVerificationFailsForTheWrongSignerOrPolicy(t *testing.T) {
	tr := newTrust(t)
	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		env.sign(t, env.record, tr.keyless)
		tests := map[string]artifact.VerificationPolicy{
			"wrong identity": identityPolicy("https://github.com/acme/other/.github/workflows/release.yml@refs/heads/main", trustedIssuer),
			"wrong issuer":   identityPolicy(trustedIdentity, "https://accounts.example.com"),
			"untrusted key":  keyPolicy("not-configured"),
			"two sets, one unmet": {RequireSignature: true, TrustSets: []artifact.TrustSet{
				{Identities: []artifact.TrustedIdentity{{Identity: trustedIdentity, Issuer: trustedIssuer}}},
				{Identities: []artifact.TrustedIdentity{{Identity: "someone@example.com", Issuer: trustedIssuer}}},
			}},
		}
		for name, policy := range tests {
			t.Run(name, func(t *testing.T) { env.requireRejected(t, env.record, policy) })
		}
		wildcard := identityPolicy("https://github.com/acme/images/*", trustedIssuer)
		env.requireVerified(t, env.record, wildcard)
	})
}

func TestConfiguredPublicKeyVerifiesButNeverSatisfiesAnIdentityPolicy(t *testing.T) {
	tr := newTrust(t)
	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		// Sign first so the operator-configured key can be learned, then trust exactly that key.
		repo := env.repo
		_, err := repo.Sign(context.Background(), env.record, tr.keyed, artifact.SignOptions{})
		require.NoError(t, err)

		trusting := tr.verifier(sigstore.WithTrustedKey(signaturetest.KeyHint, tr.keyBlob.publicKey()))
		repo.settings.verifier = trusting
		env.requireVerified(t, env.record, keyPolicy(signaturetest.KeyHint))
		env.requireRejected(t, env.record, identityPolicy(trustedIdentity, trustedIssuer))
		env.requireRejected(t, env.record, artifact.VerificationPolicy{RequireSignature: true, TrustSets: []artifact.TrustSet{
			{Identities: []artifact.TrustedIdentity{{Identity: trustedIdentity, Issuer: trustedIssuer}}},
			{Keys: []string{signaturetest.KeyHint}},
		}})

		// A different key is not the configured one.
		other := sigstore.NewVerifier(tr.loader(tr.auth), sigstore.WithTrustedKey(signaturetest.KeyHint, newOtherKey(t, tr)))
		repo.settings.verifier = other
		env.requireRejected(t, env.record, keyPolicy(signaturetest.KeyHint))

		// A keyless signature never satisfies a key policy either.
		repo.settings.verifier = trusting
		_, err = repo.Sign(context.Background(), env.rootSub, tr.keyless, artifact.SignOptions{})
		require.NoError(t, err)
		env.requireRejected(t, env.rootSub, keyPolicy(signaturetest.KeyHint))
	})
}

// newOtherKey returns the public key of an unrelated signer.
func newOtherKey(t *testing.T, tr *trust) crypto.PublicKey {
	t.Helper()
	return tr.auth.SignWithKey(t, []byte("unrelated")).PublicKey
}

func TestMissingOrExpiredTrustFailsPolicyOffline(t *testing.T) {
	tr := newTrust(t)
	cases := map[string]sigstore.VerifierOption{
		"offline with no cached roots": sigstore.WithRootLoader(func(_ context.Context, offline bool) (sigroot.TrustedMaterial, error) {
			require.True(t, offline, "an offline policy asks the loader for offline trust")
			return nil, errUtils.ErrSignatureOffline
		}),
		"roots of another authority": tr.loader(signaturetest.New(t)),
		"loader failure": sigstore.WithRootLoader(func(context.Context, bool) (sigroot.TrustedMaterial, error) {
			return nil, errUtils.ErrSignatureInvalid
		}),
	}
	for name, option := range cases {
		t.Run(name, func(t *testing.T) {
			withVerifier(t, sigstore.NewVerifier(option), func(t *testing.T, env *signedEnv) {
				// Sign with an unrestricted verifier so the evidence exists, then verify offline.
				env.repo.settings.verifier = tr.verifier()
				env.sign(t, env.record, tr.keyless)
				env.repo.settings.verifier = sigstore.NewVerifier(option)
				policy := identityPolicy(trustedIdentity, trustedIssuer)
				policy.Offline = true
				env.requireRejected(t, env.record, policy)
			})
		})
	}
}

// attach stores bundle bytes as evidence bound to the subject, bypassing the signer.
func (e *signedEnv) attach(t *testing.T, subject artifact.SignatureSubject, data []byte) {
	t.Helper()
	pinned := mustResolveRef(t, e.repo, artifact.Ref{Name: appName, Version: appVersion})
	st, loaded, err := e.repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	payload := &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: bundleMediaTypePrefix + "v0.3+json", Data: data}
	_, _, err = e.repo.storeAttachment(context.Background(), st, &attachRequest{root: loaded.desc, subject: subject, payload: payload, configDigest: loaded.manifest.Config.Digest})
	require.NoError(t, err)
}

func TestTamperedWrongSubjectAndMalformedEvidence(t *testing.T) {
	tr := newTrust(t)
	policy := identityPolicy(trustedIdentity, trustedIssuer)
	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		valid, err := tr.keyless.Sign(context.Background(), env.record)
		require.NoError(t, err)

		t.Run("tampered signature", func(t *testing.T) {
			var bundle map[string]any
			require.NoError(t, json.Unmarshal(valid.Data, &bundle))
			message, ok := bundle["messageSignature"].(map[string]any)
			require.True(t, ok)
			message["signature"] = base64.StdEncoding.EncodeToString([]byte("forged"))
			tampered, err := json.Marshal(bundle)
			require.NoError(t, err)
			env.attach(t, env.record, tampered)
			env.requireRejected(t, env.record, policy)
		})
		t.Run("signature made for another subject", func(t *testing.T) {
			other := env.record
			other.Digest = digest.FromString("another record")
			foreign, err := tr.keyless.Sign(context.Background(), other)
			require.NoError(t, err)
			env.attach(t, env.record, foreign.Data)
			env.requireRejected(t, env.record, policy)
		})
		t.Run("unrelated invalid evidence never overrides a valid signature", func(t *testing.T) {
			env.attach(t, env.record, []byte("not a bundle at all"))
			env.attach(t, env.record, []byte(`{"mediaType":"application/vnd.dev.sigstore.bundle.v0.3+json"}`))
			env.requireRejected(t, env.record, policy)
			env.attach(t, env.record, valid.Data)
			result := env.requireVerified(t, env.record, policy)
			codes := map[string]bool{}
			for _, diagnostic := range result.Diagnostics {
				codes[diagnostic.Code] = true
			}
			assert.True(t, codes["malformed-evidence"], "malformed attachments are reported, not hidden: %v", result.Diagnostics)
			assert.LessOrEqual(t, len(result.Diagnostics), 17, "reporting is bounded")
		})
	})
}

func TestSignatureBoundToAnotherManifestIsIgnored(t *testing.T) {
	tr := newTrust(t)
	withVerifier(t, tr.verifier(), func(t *testing.T, env *signedEnv) {
		other := publish(t, env.repo, publishSpec{name: "other/app", content: "other"})
		otherPinned := mustResolveRef(t, env.repo, artifact.Ref{Name: "other/app", Version: appVersion})
		_, err := env.repo.Sign(context.Background(), artifact.SignatureSubject{
			Kind: artifact.SubjectRecord, Digest: other.Pinned.RecordDigest, MediaType: artifact.MediaTypeConfig, Size: otherPinnedSize(t, env.repo, otherPinned),
		}, tr.keyless, artifact.SignOptions{})
		require.NoError(t, err)

		// The other package's valid signature is not evidence for this one.
		result := env.requireRejected(t, env.record, identityPolicy(trustedIdentity, trustedIssuer))
		assert.NotEmpty(t, result.Diagnostics)
	})
}

func otherPinnedSize(t *testing.T, repo *Repository, pinned artifact.PinnedReference) int64 {
	t.Helper()
	_, loaded, err := repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	return loaded.manifest.Config.Size
}

func TestSubjectsMustBeResolvedBeforeTheyCanBeSignedOrVerified(t *testing.T) {
	tr := newTrust(t)
	reg := newRegistry(t, nil)
	writer := newRegistryRepo(t, reg, WithVerifier(tr.verifier()))
	result := publish(t, writer, publishSpec{content: "fresh"})
	fresh := newRegistryRepo(t, reg, WithVerifier(tr.verifier()))
	subject := artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: result.Pinned.RecordDigest, MediaType: artifact.MediaTypeConfig}

	_, err := fresh.Verify(context.Background(), subject, identityPolicy(trustedIdentity, trustedIssuer))
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	assert.Contains(t, err.Error(), "resolve")
	mustResolveRef(t, fresh, artifact.Ref{Name: appName, Digest: result.Pinned.RecordDigest})
	_, err = fresh.Verify(context.Background(), subject, identityPolicy(trustedIdentity, trustedIssuer))
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "resolved, so it is checked and found unsigned")
}

func TestNativeImagePluginAndLegacyCosignSignatures(t *testing.T) {
	tr := newTrust(t)
	ctx := context.Background()
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg, WithVerifier(tr.verifier()))
	policy := identityPolicy(trustedIdentity, trustedIssuer)

	t.Run("image signed through the signer", func(t *testing.T) {
		image := pushImage(t, reg, testPrefix+"/tools/img", "v1")
		pinned := mustResolveRef(t, repo, artifact.Ref{Name: "tools/img", Version: "v1"})
		subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: image.manifest, MediaType: "application/vnd.oci.image.manifest.v1+json"}
		_, err := repo.Verify(ctx, subject, policy)
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed)
		_, err = repo.Sign(ctx, subject, tr.keyless, artifact.SignOptions{})
		require.NoError(t, err)
		result, err := repo.Verify(ctx, subject, policy)
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationVerified, result.Status)
		_, err = repo.Sign(ctx, artifact.SignatureSubject{Kind: artifact.SubjectRecord, Digest: image.manifest}, tr.keyless, artifact.SignOptions{})
		require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid, "a foreign image has no Record to sign")
		assert.NotEmpty(t, pinned.ResolvedDigest)
	})

	t.Run("plugin release signed over its raw index like sign-blob", func(t *testing.T) {
		ref := artifact.Ref{Name: "plugins/example", Version: "1.2.3"}
		importPlugin(t, repo, ref)
		pinned := mustResolveRef(t, repo, ref)
		raw := rawRootBytes(t, repo, pinned)
		bundle := tr.auth.Sign(t, trustedIdentity, trustedIssuer, raw)
		subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: pluginRoot, MediaType: "application/vnd.oci.image.index.v1+json", Size: int64(len(raw))}
		attachBundleRaw(t, repo, pinned, subject, bundle)
		result, err := repo.Verify(ctx, subject, policy)
		require.NoError(t, err, "the existing plugin signing form still verifies")
		assert.Equal(t, artifact.VerificationVerified, result.Status)
	})

	t.Run("cosign 3 style DSSE statement", func(t *testing.T) {
		image := pushImage(t, reg, testPrefix+"/tools/dsse", "v1")
		pinned := mustResolveRef(t, repo, artifact.Ref{Name: "tools/dsse", Version: "v1"})
		subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: image.manifest, MediaType: "application/vnd.oci.image.manifest.v1+json"}
		bundle := tr.auth.SignStatement(t, trustedIdentity, trustedIssuer, rawRootBytes(t, repo, pinned))
		attachBundleRaw(t, repo, pinned, subject, bundle)
		result, err := repo.Verify(ctx, subject, policy)
		require.NoError(t, err)
		assert.Equal(t, artifact.VerificationVerified, result.Status)
	})

	t.Run("legacy cosign tag payloads are not evidence", func(t *testing.T) {
		image := pushImage(t, reg, testPrefix+"/tools/legacy", "v1")
		repoName := reg.server.Host + "/" + testPrefix + "/tools/legacy"
		signatureTag := "sha256-" + image.manifest.Encoded() + ".sig"
		reference, err := name.ParseReference(repoName + ":" + signatureTag)
		require.NoError(t, err)
		payload, err := random.Image(32, 1)
		require.NoError(t, err)
		require.NoError(t, remote.Write(reference, payload, remote.WithTransport(reg.recorder)))
		mustResolveRef(t, repo, artifact.Ref{Name: "tools/legacy", Version: "v1"})
		subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: image.manifest, MediaType: "application/vnd.oci.image.manifest.v1+json"}
		_, err = repo.Verify(ctx, subject, policy)
		require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "unsupported required evidence fails closed")
	})
}

// attachBundleRaw attaches bundle bytes produced outside the signer as evidence of the root.
func attachBundleRaw(t *testing.T, repo *Repository, pinned artifact.PinnedReference, subject artifact.SignatureSubject, bundle []byte) {
	t.Helper()
	st, loaded, err := repo.rootOf(context.Background(), &pinned)
	require.NoError(t, err)
	payload := &artifact.EvidencePayload{Format: artifact.EvidenceSigstoreBundle, MediaType: bundleMediaTypePrefix + "v0.3+json", Data: bundle}
	_, _, err = repo.storeAttachment(context.Background(), st, &attachRequest{root: loaded.desc, subject: subject, payload: payload})
	require.NoError(t, err)
}

// A signed multi-platform root keeps its signature when a child is selected; an index rebuilt
// around that child is a different object and claims none of it.
func TestOriginalIndexSignatureSurvivesPlatformSelectionAndReconstructionClaimsNone(t *testing.T) {
	tr := newTrust(t)
	ctx := context.Background()
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg, WithVerifier(tr.verifier()))
	policy := identityPolicy(trustedIdentity, trustedIssuer)
	image := pushIndex(t, reg, testPrefix+"/tools/img", "v1", multiPlatform...)
	pinned := mustResolveRef(t, repo, artifact.Ref{Name: "tools/img", Version: "v1"})
	subject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: image.manifest, MediaType: "application/vnd.oci.image.index.v1+json"}
	_, err := repo.Sign(ctx, subject, tr.keyless, artifact.SignOptions{})
	require.NoError(t, err)

	selected, child, err := repo.SelectPlatform(ctx, pinned, ocispec.Platform{OS: multiPlatform[1].OS, Architecture: multiPlatform[1].Architecture})
	require.NoError(t, err)
	inspection, err := repo.Inspect(ctx, selected)
	require.NoError(t, err)
	require.NotNil(t, inspection.Representation.OriginalRoot)
	signed := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: inspection.Representation.OriginalRoot.Digest, MediaType: inspection.Representation.OriginalRoot.MediaType}
	result, err := repo.Verify(ctx, signed, policy)
	require.NoError(t, err, "the selected child is verified through the original signed index")
	assert.Equal(t, artifact.VerificationVerified, result.Status)
	assert.Len(t, inspection.Evidence, 1)

	// Rebuild an index that holds only the selected child and publish it under another tag.
	reconstructed := reconstructedIndex(t, reg, image, child.Digest.String())
	assert.NotEqual(t, image.manifest, reconstructed, "a reconstructed root has its own digest")
	narrow := mustResolveRef(t, repo, artifact.Ref{Name: "tools/img", Version: "narrow"})
	assert.Equal(t, reconstructed, narrow.ResolvedDigest)
	narrowSubject := artifact.SignatureSubject{Kind: artifact.SubjectOCIRoot, Digest: reconstructed, MediaType: "application/vnd.oci.image.index.v1+json"}
	_, err = repo.Verify(ctx, narrowSubject, policy)
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "the rebuilt root does not inherit the original signature")
}

// reconstructedIndex pushes tools/img:narrow holding only the child manifest and returns its digest.
func reconstructedIndex(t *testing.T, reg *registry, image foreignImage, childDigest string) digest.Digest {
	t.Helper()
	repoName := reg.server.Host + "/" + testPrefix + "/tools/img"
	childReference, err := name.NewDigest(repoName + "@" + childDigest)
	require.NoError(t, err)
	child, err := remote.Image(childReference, remote.WithTransport(reg.recorder))
	require.NoError(t, err)
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: child})
	reference, err := name.ParseReference(repoName + ":narrow")
	require.NoError(t, err)
	require.NoError(t, remote.WriteIndex(reference, index, remote.WithTransport(reg.recorder)))
	rebuilt, err := index.Digest()
	require.NoError(t, err)
	assert.NotEmpty(t, image.children)
	return digest.Digest(rebuilt.String())
}
