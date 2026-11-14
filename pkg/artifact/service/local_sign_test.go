package service_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the schema fields the local repository config uses.
var _ = schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": ""}}

// gatedSigner signs like [artifacttest.FakeSigner] but announces when it starts and waits to be
// released, so a test can run another operation while a signature is in flight.
type gatedSigner struct {
	artifacttest.FakeSigner
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedSigner() *gatedSigner {
	return &gatedSigner{
		FakeSigner: artifacttest.FakeSigner{Identity: "release@example.com", Issuer: "https://issuer.example.com"},
		started:    make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (g *gatedSigner) Sign(ctx context.Context, subject artifact.SignatureSubject) (*artifact.EvidencePayload, error) {
	g.once.Do(func() { close(g.started) })
	select {
	case <-g.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.FakeSigner.Sign(ctx, subject)
}

// localSignEnv is a service over a real local directory repository.
type localSignEnv struct {
	svc       *service.Service
	repo      *local.Repository
	published *service.PublishOutcome
	digestRef artifact.Ref
}

func newLocalSignEnv(t *testing.T, name string, signer artifact.Signer) *localSignEnv {
	t.Helper()
	cfg := &schema.ArtifactRepositoryConfig{
		Kind:    artifact.RepositoryKindLocalDir,
		Options: map[string]any{"path": filepath.Join(t.TempDir(), "repo")},
	}
	repo, err := local.NewDir("dst", cfg, local.WithVerifier(&artifacttest.FakeVerifier{}), local.WithOwner("local-test"))
	require.NoError(t, err)
	static := &service.StaticRepositories{Repositories: map[string]artifact.Repository{"dst": repo}, Info: map[string]service.RepositoryInfo{}}
	svc, err := service.New(static, nil, service.WithSigner(signer), service.WithOwner("service-test"), service.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)

	src := (&env{t: t}).source("hello")
	published, err := svc.PublishDetailed(context.Background(), artifact.PublishRequest{Repository: "dst", Source: src, Name: name, Version: "1.0.0"})
	require.NoError(t, err)
	return &localSignEnv{svc: svc, repo: repo, published: published, digestRef: artifact.Ref{Name: name, Digest: published.Operation.RecordDigest}}
}

func (e *localSignEnv) sign(ctx context.Context) (*artifact.SignResult, error) {
	return e.svc.Sign(ctx, artifact.SignRequest{Repository: "dst", Ref: e.digestRef})
}

// Signing through the service must work against a real local repository: the lock the service
// holds has to be one the repository accepts. Names cover a reference that sorts before and after
// the "subject:" lock resource, because the service takes its locks in canonical order.
func TestSign_ThroughTheServiceStoresEvidenceInALocalRepository(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "reference sorts after the subject lock", ref: "tools/example"},
		{name: "reference sorts before the subject lock", ref: "a/example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newLocalSignEnv(t, tt.ref, releaseSigner)

			result, err := e.sign(context.Background())
			require.NoError(t, err)
			require.Len(t, result.Evidence, 1)
			assert.Equal(t, artifact.SubjectRecord, result.Subject.Kind)
			assert.Equal(t, e.published.Operation.RecordDigest, result.Subject.Digest)
			assert.Equal(t, artifact.LockScopeHost, result.Operation.LockScope)

			inspected, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: artifact.Ref{Name: tt.ref, Version: "1.0.0"}})
			require.NoError(t, err)
			require.Len(t, inspected.Inspection.Evidence, 1)
			assert.Equal(t, result.Evidence[0].Descriptor.Digest, inspected.Inspection.Evidence[0].Descriptor.Digest)

			verified, err := e.svc.Verify(context.Background(), artifact.VerifyRequest{Repository: "dst", Ref: e.digestRef})
			require.Error(t, err, "no trust configuration is set, so verification reports unchecked rather than green")
			assert.Equal(t, artifact.VerificationUnchecked, verified.Verification.Status)

			// Every lock the service took was released: a second signature and a delete both proceed.
			_, err = e.sign(context.Background())
			require.NoError(t, err)
			_, err = e.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: artifact.Ref{Name: tt.ref, Version: "1.0.0"}})
			require.NoError(t, err)
		})
	}
}

// While a signer is working, a concurrent delete through the service must wait for the signature
// to be stored instead of removing the Record underneath it, and it must not deadlock with the
// signing locks whichever way the reference sorts against the subject lock.
func TestSign_ThroughTheServiceExcludesAConcurrentDelete(t *testing.T) {
	for _, name := range []string{"tools/example", "a/example"} {
		t.Run(name, func(t *testing.T) {
			signer := newGatedSigner()
			e := newLocalSignEnv(t, name, signer)

			signed := make(chan error, 1)
			go func() {
				_, err := e.sign(context.Background())
				signed <- err
			}()
			<-signer.started

			deleted := make(chan error, 1)
			go func() {
				_, err := e.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "dst", Ref: artifact.Ref{Name: name, Version: "1.0.0"}})
				deleted <- err
			}()

			select {
			case err := <-deleted:
				t.Fatalf("delete finished while the signer was still working: %v", err)
			case <-time.After(200 * time.Millisecond):
			}

			close(signer.release)
			require.NoError(t, waitFor(t, signed), "signing completes once the signer returns")
			require.NoError(t, waitFor(t, deleted), "the delete runs after the signature is stored")

			_, err := e.svc.Inspect(context.Background(), artifact.InspectRequest{Repository: "dst", Ref: artifact.Ref{Name: name, Version: "1.0.0"}})
			require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
		})
	}
}

// A writer that bypasses the service and deletes through the repository takes the repository's own
// reference lock, which signing through the service must also hold.
func TestSign_ThroughTheServiceExcludesADirectRepositoryDelete(t *testing.T) {
	signer := newGatedSigner()
	e := newLocalSignEnv(t, "tools/example", signer)

	signed := make(chan error, 1)
	go func() {
		_, err := e.sign(context.Background())
		signed <- err
	}()
	<-signer.started

	pinned, err := e.repo.Resolve(context.Background(), artifact.Ref{Name: "tools/example", Version: "1.0.0"})
	require.NoError(t, err)
	deleted := make(chan error, 1)
	go func() {
		_, deleteErr := e.repo.Delete(context.Background(), pinned, artifact.DeleteOptions{})
		deleted <- deleteErr
	}()

	select {
	case err := <-deleted:
		t.Fatalf("a direct delete finished while the signer was still working: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(signer.release)
	require.NoError(t, waitFor(t, signed))
	require.NoError(t, waitFor(t, deleted))
}

func waitFor(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the operation to finish")
		return nil
	}
}
