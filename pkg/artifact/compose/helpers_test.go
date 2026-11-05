package compose_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	sigroot "github.com/sigstore/sigstore-go/pkg/root"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	blobs3 "github.com/cloudposse/atmos/pkg/artifact/blob/s3"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/artifact/s3"
	"github.com/cloudposse/atmos/pkg/artifact/sigstore"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/oci/ocitest"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signature/signaturetest"
)

// Compile-time guards for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: "", Identity: "", Locking: &schema.ArtifactLockingConfig{}}

const (
	appName    = "team/app"
	appVersion = "1.0.0"
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

// countingVerifier wraps the real Sigstore verifier and counts calls, so a test can prove that
// every repository kind uses the one shared instance.
type countingVerifier struct {
	artifact.Verifier
	calls atomic.Int64
}

func (c *countingVerifier) Verify(ctx context.Context, subject artifact.SignatureSubject, evidence []artifact.EvidenceBlob, policy artifact.VerificationPolicy) (*artifact.VerificationResult, error) {
	c.calls.Add(1)
	return c.Verifier.Verify(ctx, subject, evidence, policy)
}

// trustFixture is one isolated Sigstore trust domain with a signer and a counting verifier.
type trustFixture struct {
	signer   compose.Option
	verifier *countingVerifier
	option   compose.Option
}

func newTrustFixture(t *testing.T) *trustFixture {
	t.Helper()
	auth := signaturetest.New(t)
	real := sigstore.NewVerifier(sigstore.WithRootLoader(func(context.Context, bool) (sigroot.TrustedMaterial, error) { return auth, nil }))
	counting := &countingVerifier{Verifier: real}
	return &trustFixture{
		signer:   compose.WithBlobSigner(keylessBlob{t: t, auth: auth, identity: trustedIdentity, issuer: trustedIssuer}),
		verifier: counting,
		option:   compose.WithVerifier(counting),
	}
}

// trustSigner is the verification policy that only the fixture's signer satisfies.
func trustSigner() *schema.ArtifactVerificationConfig {
	return &schema.ArtifactVerificationConfig{
		TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: trustedIdentity, Issuer: trustedIssuer}},
	}
}

// anonymous resolves every registry anonymously, never reading the developer's Docker configuration.
type anonymous struct{}

func (anonymous) Resolve(authn.Resource) (authn.Authenticator, error) { return authn.Anonymous, nil }

// testRegistry is an in-process registry plus a transport factory bound to it.
type testRegistry struct {
	host     string
	requests atomic.Int64
	factory  oci.TransportFactory
}

func newTestRegistry(t *testing.T) *testRegistry {
	t.Helper()
	server := ocitest.NewServerWithOptions(t, nil, ocitest.WithTagsFollowManifestDelete())
	reg := &testRegistry{host: server.Host}
	reg.factory = func(context.Context, string, *schema.ArtifactRepositoryConfig, artifact.IdentityResolver) (atmosoci.Transport, error) {
		reg.requests.Add(1)
		return atmosoci.NewClient(nil, atmosoci.WithKeychain(anonymous{})), nil
	}
	return reg
}

// s3Fixture is a fake S3 endpoint and the store option that points an aws/s3 repository at it.
type s3Fixture struct {
	server *s3test.Server
	option s3.Option
}

func newS3Fixture(t *testing.T) *s3Fixture {
	t.Helper()
	server := s3test.NewServer(t)
	store, err := blobs3.NewFromClient(server.Client(), server.Bucket(), blobs3.WithPartSize(16<<10))
	require.NoError(t, err)
	return &s3Fixture{server: server, option: s3.WithStore(store)}
}

func (f *s3Fixture) repository() schema.ArtifactRepositoryConfig {
	return schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindS3, Options: map[string]any{"bucket": f.server.Bucket()}}
}

// source writes a small package and returns its directory.
func source(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "data.txt", content, 0o644)
	return dir
}

func publishRequest(repository, src string) artifact.PublishRequest {
	return artifact.PublishRequest{Repository: repository, Source: src, Name: appName, Version: appVersion}
}

func appRef() artifact.Ref { return artifact.Ref{Name: appName, Version: appVersion} }

// localRepo returns a local repository configuration rooted below a temporary directory.
func localRepo(kind, dir string) schema.ArtifactRepositoryConfig {
	return schema.ArtifactRepositoryConfig{Kind: kind, Options: map[string]any{"path": dir}}
}

// layoutRepo returns an OCI layout repository configuration rooted below a temporary directory.
func layoutRepo(dir string) schema.ArtifactRepositoryConfig {
	return schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindOCILayout, Options: map[string]any{"path": filepath.Join(dir, "layout")}}
}

// build composes a service from repositories and the shared fakes.
func build(t *testing.T, repos map[string]schema.ArtifactRepositoryConfig, verification *schema.ArtifactVerificationConfig, opts ...compose.Option) *compose.Composition {
	t.Helper()
	config := &schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: repos, Verification: verification}}
	composition, err := compose.New(config, opts...)
	require.NoError(t, err)
	return composition
}

// newCountingFactory returns an offline transport factory that counts constructions.
func newCountingFactory(counter *atomic.Int64) oci.TransportFactory {
	return func(context.Context, string, *schema.ArtifactRepositoryConfig, artifact.IdentityResolver) (atmosoci.Transport, error) {
		counter.Add(1)
		return atmosoci.NewClient(nil, atmosoci.WithKeychain(anonymous{})), nil
	}
}
