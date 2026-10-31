package oci

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
)

// bundleSigner wraps the conformance fake signer so its evidence carries a Sigstore bundle media
// type, the only attachment type OCI repositories store.
type bundleSigner struct{ *artifacttest.FakeSigner }

func (s bundleSigner) Sign(ctx context.Context, subject artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	payload, err := s.FakeSigner.Sign(ctx, subject)
	if payload != nil {
		payload.MediaType = bundleMediaTypePrefix + "v0.3+json"
	}
	return payload, err
}

func conformanceSigner() (bundleSigner, *artifacttest.FakeSigner) {
	fake := &artifacttest.FakeSigner{Identity: signerID, Issuer: signerIssue}
	return bundleSigner{fake}, fake
}

func TestRegistryRepositoryConformance(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingTgz, artifact.PackagingTar, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			signer, fake := conformanceSigner()
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New: func(t *testing.T) artifact.Repository {
					return newRegistryRepo(t, newRegistry(t, nil), WithVerifier(&artifacttest.FakeVerifier{}))
				},
				Packaging:       packaging,
				Signer:          signer,
				TrustedPolicy:   fake.TrustedPolicy(),
				UntrustedPolicy: fake.UntrustedPolicy(),
			})
		})
	}
}

func TestLayoutRepositoryConformance(t *testing.T) {
	for _, packaging := range []artifact.Packaging{artifact.PackagingTgz, artifact.PackagingZip} {
		t.Run(string(packaging), func(t *testing.T) {
			signer, fake := conformanceSigner()
			artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
				New: func(t *testing.T) artifact.Repository {
					return newLayoutRepo(t, WithVerifier(&artifacttest.FakeVerifier{}))
				},
				Packaging:       packaging,
				Signer:          signer,
				TrustedPolicy:   fake.TrustedPolicy(),
				UntrustedPolicy: fake.UntrustedPolicy(),
			})
		})
	}
}

// Without a verifier the repositories advertise no evidence formats, and Sign returns a
// capability error instead of a successful no-op.
func TestRepositoryConformance_WithoutVerifier(t *testing.T) {
	t.Run("registry", func(t *testing.T) {
		artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
			New: func(t *testing.T) artifact.Repository { return newRegistryRepo(t, newRegistry(t, nil)) },
		})
	})
	t.Run("layout", func(t *testing.T) {
		artifacttest.RunRepositoryConformance(t, &artifacttest.Harness{
			New: func(t *testing.T) artifact.Repository { return newLayoutRepo(t) },
		})
	})
}

func TestCapabilitiesAreHonestPerBackend(t *testing.T) {
	registryCaps := newRegistryRepo(t, newRegistry(t, nil)).Capabilities()
	layoutCaps := newLayoutRepo(t).Capabilities()

	require.Equal(t, artifact.DeleteManifest, registryCaps.Delete, "a registry deletes manifests, never a lone tag")
	require.Equal(t, artifact.DeleteReference, layoutCaps.Delete)
	for _, caps := range []artifact.Capabilities{registryCaps, layoutCaps} {
		require.False(t, caps.ConditionalUpdate, "OCI tags have no compare-and-swap")
		require.True(t, caps.HasLockScope(artifact.LockScopeHost))
		require.False(t, caps.HasLockScope(artifact.LockScopeRepository), "a tag or host guard is never advertised as a distributed lock")
		require.True(t, caps.EnumerateNamed)
		require.Empty(t, caps.EvidenceFormats, "without a verifier no evidence format is claimed")
	}
	require.True(t, registryCaps.EnumerateCollection, "the default client supports the repository catalog")
	require.True(t, layoutCaps.EnumerateCollection)
}
