package catalog_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/internal/testkit"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

// lockSpy wraps a repository and observes (or fails) its coordinator.
type lockSpy struct {
	artifact.Repository
	lockErr    error
	releaseErr error
	locks      atomic.Int64
	releases   atomic.Int64
	resources  []string
}

func (l *lockSpy) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	l.locks.Add(1)
	l.resources = append(l.resources, request.Resource)
	if l.lockErr != nil {
		return nil, l.lockErr
	}
	handle, err := l.Repository.Lock(ctx, request)
	if err != nil {
		return nil, err
	}
	return &spyHandle{LockHandle: handle, spy: l}, nil
}

type spyHandle struct {
	artifact.LockHandle
	spy *lockSpy
}

func (h *spyHandle) Release(ctx context.Context) error {
	h.spy.releases.Add(1)
	err := h.LockHandle.Release(ctx)
	if h.spy.releaseErr != nil {
		return h.spy.releaseErr
	}
	return err
}

func withRepos(repos map[string]artifact.Repository, infos map[string]service.RepositoryInfo, opts ...catalog.Option) *catalog.Service {
	return catalog.NewService(&service.StaticRepositories{Repositories: repos, Info: infos}, opts...)
}

func TestRepositoryWithoutACoordinatorProceedsUnguarded(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	info := e.info
	info.Capabilities.LockScopes = nil
	svc := withRepos(map[string]artifact.Repository{"shared": e.repo}, map[string]service.RepositoryInfo{"shared": info}, catalog.WithClock(func() time.Time { return now }))
	result, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	assert.Empty(t, result.LockScope, "no coordinator means external serialization, which the documentation states")
}

func TestWriteHoldsTheRepositoryLockAndReleasesIt(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	spy := &lockSpy{Repository: e.repo}
	svc := withRepos(map[string]artifact.Repository{"shared": spy}, map[string]service.RepositoryInfo{"shared": e.info},
		catalog.WithOwner("ci-runner"), catalog.WithIDGenerator(func() string { return "op-fixed" }), catalog.WithConcurrency(2))
	_, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	assert.Equal(t, int64(1), spy.locks.Load())
	assert.Equal(t, int64(1), spy.releases.Load(), "the lock is released after the write")
	assert.Equal(t, []string{"catalog/catalog:stable"}, spy.resources)
}

func TestLockFailureStopsTheWriteBeforeAnyChange(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	failure := errors.New("lock is held by another writer")
	spy := &lockSpy{Repository: e.repo, lockErr: failure}
	svc := withRepos(map[string]artifact.Repository{"shared": spy}, map[string]service.RepositoryInfo{"shared": e.info})
	_, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, failure)
	_, err = catalog.Read(context.Background(), e.repo, ref("catalog:stable"))
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "nothing was published")
}

func TestLockReleaseFailureIsReportedWithRecoveryContext(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	failure := errors.New("release refused")
	spy := &lockSpy{Repository: e.repo, releaseErr: failure}
	svc := withRepos(map[string]artifact.Repository{"shared": spy}, map[string]service.RepositoryInfo{"shared": e.info})
	result, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, failure)
	assert.Contains(t, err.Error(), "operator recovery")
	require.NotNil(t, result, "the completed write is still reported")
	assert.Equal(t, []string{"app:1.0.0"}, result.Added)
}

func TestLockDelegationUsesTheDelegateRepository(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	coordinator, err := local.NewDir("locks", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": testkit.LocalDir(t)}})
	require.NoError(t, err)
	spy := &lockSpy{Repository: coordinator}
	info := e.info
	info.LockProvider = "locks"
	svc := withRepos(
		map[string]artifact.Repository{"shared": e.repo, "locks": spy},
		map[string]service.RepositoryInfo{"shared": info, "locks": {Capabilities: coordinator.Capabilities()}},
	)
	result, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	assert.Equal(t, int64(1), spy.locks.Load(), "the delegate holds the lock")
	assert.Equal(t, int64(1), spy.releases.Load())
	require.Len(t, spy.resources, 1)
	assert.Contains(t, spy.resources[0], "catalog/catalog:stable")
	assert.NotEmpty(t, result.LockScope)

	t.Run("delegate cannot be opened", func(t *testing.T) {
		broken := withRepos(map[string]artifact.Repository{"shared": e.repo}, map[string]service.RepositoryInfo{"shared": info})
		_, err := broken.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
}

func TestReadAndWriteCapabilitiesAreRequired(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	readOnly := e.info
	readOnly.Capabilities.Write = false
	svc := withRepos(map[string]artifact.Repository{"shared": e.repo}, map[string]service.RepositoryInfo{"shared": readOnly})
	_, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)

	writeOnly := e.info
	writeOnly.Capabilities.Read = false
	svc = withRepos(map[string]artifact.Repository{"shared": e.repo}, map[string]service.RepositoryInfo{"shared": writeOnly})
	_, err = svc.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}

func TestCatalogOutsideTheRepositoryPathFailsBeforeAnyWrite(t *testing.T) {
	e := newEnv(t, true)
	info := e.info
	info.Discovery.Catalog = "oci://other.example.com/acme/atmos/catalog:stable"
	svc := withRepos(map[string]artifact.Repository{"shared": e.repo}, map[string]service.RepositoryInfo{"shared": info})
	e.registry.Reset()
	_, err := svc.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
	assert.Zero(t, e.registry.Total())
}

func TestWriteRejectsDigestReferences(t *testing.T) {
	e := newEnv(t, true)
	_, err := catalog.Write(context.Background(), e.repo, artifact.Ref{Name: "catalog", Digest: digest.Digest("sha256:" + strings.Repeat("a", 64))}, catalog.New("catalog"), &catalog.WriteOptions{Now: now, OperationID: "op"})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
}
