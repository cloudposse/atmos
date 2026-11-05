package compose_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/schema"
)

// backendFixture is one composition covering all five kinds over real adapters, with the shared
// counting verifier and the fake signer.
type backendFixture struct {
	composition *compose.Composition
	trust       *trustFixture
	aliases     []string
}

func newBackendFixture(t *testing.T) *backendFixture {
	t.Helper()
	registry := newTestRegistry(t)
	bucket := newS3Fixture(t)
	trust := newTrustFixture(t)
	repos := map[string]schema.ArtifactRepositoryConfig{
		"dir":      localRepo(artifact.RepositoryKindLocalDir, t.TempDir()),
		"archive":  localRepo(artifact.RepositoryKindLocalArchive, t.TempDir()),
		"layout":   layoutRepo(t.TempDir()),
		"bucket":   bucket.repository(),
		"registry": {Kind: artifact.RepositoryKindOCIRegistry, Options: map[string]any{"registry": registry.host, "prefix": "acme/atmos"}},
	}
	composition := build(
		t, repos, trustSigner(),
		trust.option,
		trust.signer,
		compose.WithTransportFactory(registry.factory),
		compose.WithS3Options(bucket.option),
	)
	return &backendFixture{composition: composition, trust: trust, aliases: composition.Repositories.Aliases()}
}

func TestOneVerifierAndOnePolicyServeEveryRepositoryKind(t *testing.T) {
	f := newBackendFixture(t)
	require.Len(t, f.aliases, 5)

	for _, alias := range f.aliases {
		t.Run(alias, func(t *testing.T) {
			before := f.trust.verifier.calls.Load()
			published, err := f.composition.Service.Publish(context.Background(), publishRequest(alias, source(t, "hello "+alias)))
			require.NoError(t, err)

			unsigned, err := f.composition.Service.Verify(context.Background(), artifact.VerifyRequest{Repository: alias, Ref: appRef()})
			require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "an unsigned artifact never verifies")
			assert.NotEqual(t, artifact.VerificationVerified, unsigned.Verification.Status)

			_, err = f.composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: alias, Ref: artifact.Ref{Name: appName, Digest: published.RecordDigest}})
			require.NoError(t, err)

			verified, err := f.composition.Service.Verify(context.Background(), artifact.VerifyRequest{Repository: alias, Ref: appRef()})
			require.NoError(t, err)
			assert.Equal(t, artifact.VerificationVerified, verified.Verification.Status)
			assert.Greater(t, f.trust.verifier.calls.Load(), before, "%s verified through the shared verifier instance", alias)
		})
	}
}

// Sign accepts whichever digest a user sees: the digest the repository resolved (the OCI manifest
// digest, or the Record digest for local and S3) or the Record digest, and signs the Record.
func TestSignAcceptsTheResolvedAndTheRecordDigestOnEveryKind(t *testing.T) {
	f := newBackendFixture(t)
	for _, alias := range f.aliases {
		t.Run(alias, func(t *testing.T) {
			published, err := f.composition.Service.Publish(context.Background(), publishRequest(alias, source(t, "hello "+alias)))
			require.NoError(t, err)
			require.NotEmpty(t, published.ResolvedDigest)
			require.NotEmpty(t, published.RecordDigest)
			isOCI := alias == "layout" || alias == "registry"
			if isOCI {
				require.NotEqual(t, published.ResolvedDigest, published.RecordDigest, "an OCI package is addressed by its manifest, not its Record")
			}

			byResolved, err := f.composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: alias, Ref: artifact.Ref{Name: appName, Digest: published.ResolvedDigest}})
			require.NoError(t, err, "the digest users see signs the artifact")
			assert.Equal(t, artifact.SubjectRecord, byResolved.Subject.Kind)
			assert.Equal(t, published.RecordDigest, byResolved.Subject.Digest, "the signed subject is the Record")
			assert.Equal(t, published.ResolvedDigest, byResolved.Operation.ResolvedDigest)
			assert.Equal(t, published.RecordDigest, byResolved.Operation.RecordDigest)

			byRecord, err := f.composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: alias, Ref: artifact.Ref{Name: appName, Digest: published.RecordDigest}})
			require.NoError(t, err)
			assert.Equal(t, byResolved.Subject, byRecord.Subject, "either digest signs the same subject")

			verified, err := f.composition.Service.Verify(context.Background(), artifact.VerifyRequest{Repository: alias, Ref: appRef()})
			require.NoError(t, err)
			assert.Equal(t, artifact.VerificationVerified, verified.Verification.Status)
		})
	}
}

func TestSignRejectsADigestOfNothingTheUserNamed(t *testing.T) {
	f := newBackendFixture(t)
	published, err := f.composition.Service.Publish(context.Background(), publishRequest("dir", source(t, "hello")))
	require.NoError(t, err)
	other, err := f.composition.Service.Publish(context.Background(), artifact.PublishRequest{Repository: "dir", Source: source(t, "other"), Name: "team/other", Version: "1"})
	require.NoError(t, err)

	_, err = f.composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: "dir", Ref: artifact.Ref{Name: appName, Digest: other.RecordDigest}})
	require.Error(t, err, "another artifact's digest never signs this one")
	_, err = f.composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: "dir", Ref: appRef()})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid, "sign accepts digest references only")
	assert.NotEqual(t, published.RecordDigest, other.RecordDigest)
}

func TestVerificationPolicyComesFromConfigurationAndRepositoriesCannotWeakenIt(t *testing.T) {
	repos := map[string]schema.ArtifactRepositoryConfig{
		"strict": {
			Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()},
			Verification: &schema.ArtifactVerificationConfig{TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: "other@example.com", Issuer: "https://other.example.com"}}},
		},
	}
	trust := newTrustFixture(t)
	composition := build(t, repos, trustSigner(), trust.option, trust.signer)
	published, err := composition.Service.Publish(context.Background(), publishRequest("strict", source(t, "hello")))
	require.NoError(t, err)
	_, err = composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: "strict", Ref: artifact.Ref{Name: appName, Digest: published.RecordDigest}})
	require.NoError(t, err)

	_, err = composition.Service.Verify(context.Background(), artifact.VerifyRequest{Repository: "strict", Ref: appRef()})
	require.ErrorIs(t, err, errUtils.ErrArtifactVerificationFailed, "the global trust set is satisfied but the repository's own trust set is not: both are required")
}

func TestSigningNeedsASigner(t *testing.T) {
	repos := map[string]schema.ArtifactRepositoryConfig{"dir": localRepo(artifact.RepositoryKindLocalDir, t.TempDir())}
	composition := build(t, repos, trustSigner(), newTrustFixture(t).option)
	published, err := composition.Service.Publish(context.Background(), publishRequest("dir", source(t, "hello")))
	require.NoError(t, err)

	_, err = composition.Service.Sign(context.Background(), artifact.SignRequest{Repository: "dir", Ref: artifact.Ref{Name: appName, Digest: published.RecordDigest}})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		repos map[string]schema.ArtifactRepositoryConfig
	}{
		{"unknown kind", map[string]schema.ArtifactRepositoryConfig{"x": {Kind: "ftp/server"}}},
		{"missing path", map[string]schema.ArtifactRepositoryConfig{"x": {Kind: artifact.RepositoryKindLocalDir}}},
		{"delegation cycle", map[string]schema.ArtifactRepositoryConfig{
			"a": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()}, Locking: &schema.ArtifactLockingConfig{Repository: "b"}},
			"b": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()}, Locking: &schema.ArtifactLockingConfig{Repository: "a"}},
		}},
		{"delegate lacks the required scope", map[string]schema.ArtifactRepositoryConfig{
			"a": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()}, Locking: &schema.ArtifactLockingConfig{RequiredScope: "repository", Repository: "b"}},
			"b": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := compose.New(&schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: tt.repos}})
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
		})
	}
}

func TestNewAcceptsAMissingConfiguration(t *testing.T) {
	for _, config := range []*schema.AtmosConfiguration{nil, {}} {
		composition, err := compose.New(config)
		require.NoError(t, err)
		assert.Empty(t, composition.Repositories.Aliases())
		_, err = composition.Service.Pull(context.Background(), artifact.PullRequest{Repository: "missing", Ref: appRef(), Destination: t.TempDir()})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	}
}

func TestRegisteringIntoAnOccupiedRegistryFailsLoudly(t *testing.T) {
	registry := artifact.NewRepositoryRegistry()
	_, err := compose.New(&schema.AtmosConfiguration{}, compose.WithRegistry(registry))
	require.NoError(t, err)
	_, err = compose.New(&schema.AtmosConfiguration{}, compose.WithRegistry(registry))
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "double registration is caught, not silently replaced")
}

// The composition wires the plugin-release immutability policy: a package in the plugin format can
// never be replaced through the generic operations, even with force.
func TestPluginFormatArtifactsStayImmutableEvenWithForce(t *testing.T) {
	repos := map[string]schema.ArtifactRepositoryConfig{"layout": layoutRepo(t.TempDir())}
	composition := build(t, repos, nil)
	request := publishRequest("layout", source(t, "release"))
	request.Metadata = artifact.Metadata{Labels: map[string]string{"dev.atmos.plugin.name": "login"}}
	_, err := composition.Service.Publish(context.Background(), request)
	require.NoError(t, err)

	replacement := publishRequest("layout", source(t, "tampered"))
	replacement.Metadata = request.Metadata
	replacement.Force = true
	_, err = composition.Service.Publish(context.Background(), replacement)
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict, "force never bypasses immutable-release policy")

	inspected, err := composition.Service.Inspect(context.Background(), artifact.InspectRequest{Repository: "layout", Ref: appRef()})
	require.NoError(t, err)
	assert.NotNil(t, inspected.Inspection.Record)
}
