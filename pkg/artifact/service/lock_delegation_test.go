package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

// spyCoordinator is an in-memory repository that records the lock requests it receives and names
// its own collection, like a real backend does.
type spyCoordinator struct {
	*artifacttest.Repository
	mu       sync.Mutex
	requests []artifact.LockRequest
}

func (s *spyCoordinator) Lock(ctx context.Context, request artifact.LockRequest) (artifact.LockHandle, error) {
	s.mu.Lock()
	s.requests = append(s.requests, request)
	s.mu.Unlock()
	return s.Repository.Lock(ctx, request)
}

func (s *spyCoordinator) recorded() []artifact.LockRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]artifact.LockRequest(nil), s.requests...)
}

// namedRepository is an in-memory target that reports its collection, like local, S3, and OCI do.
type namedRepository struct {
	*artifacttest.Repository
	namespace string
}

func (n *namedRepository) LockNamespace() string { return n.namespace }

// delegatingEnv is a host-scope "work" repository whose locking is delegated to a repository-scope
// "coord" repository, wired through the service the way configuration wires it.
type delegatingEnv struct {
	*env
	work  *artifacttest.Repository
	coord *spyCoordinator
}

func newDelegatingEnv(t *testing.T, workWrap func(*artifacttest.Repository) artifact.Repository, coordScopes ...artifact.LockScope) *delegatingEnv {
	t.Helper()
	workCaps := artifacttest.DefaultCapabilities()
	workCaps.LockScopes = []artifact.LockScope{artifact.LockScopeHost}
	coordCaps := artifacttest.DefaultCapabilities()
	coordCaps.LockScopes = coordScopes
	e := newEnv(t, envConfig{
		repos:   map[string]artifacttest.Options{"work": {Capabilities: &workCaps}},
		options: []service.Option{service.WithSigner(releaseSigner)},
	})
	work := e.repos["work"]
	if workWrap != nil {
		e.static.Repositories["work"] = workWrap(work)
	}
	coord := &spyCoordinator{Repository: artifacttest.New(artifacttest.Options{Alias: "coord", Capabilities: &coordCaps})}
	e.static.Repositories["coord"] = coord
	// What ConfigRepositories.Describe reports for locking: {repository: coord, required_scope: repository}.
	delegated := workCaps
	delegated.LockScopes = append(append([]artifact.LockScope(nil), workCaps.LockScopes...), coordScopes...)
	e.static.Info["work"] = service.RepositoryInfo{Capabilities: delegated, Packaging: artifact.PackagingNone, RequiredLockScope: artifact.LockScopeRepository, LockProvider: "coord"}
	return &delegatingEnv{env: e, work: work, coord: coord}
}

var bothScopes = []artifact.LockScope{artifact.LockScopeHost, artifact.LockScopeRepository}

func TestLockDelegation_PublishLocksTheDelegateNotTheTarget(t *testing.T) {
	d := newDelegatingEnv(t, nil, bothScopes...)

	result, err := d.svc.Publish(context.Background(), d.publishRequest("work", d.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	assert.Equal(t, artifact.LockScopeRepository, result.LockScope, "the delegate's actual scope is reported")

	assert.Zero(t, d.work.Calls("Lock"), "the target repository is never asked to lock when locking is delegated")
	requests := d.coord.recorded()
	require.Len(t, requests, 1)
	assert.Equal(t, "work/tools/example:1.0.0", requests[0].Resource, "a target without a namespace is scoped by its alias")
	assert.Equal(t, artifact.LockScopeRepository, requests[0].Required)
	assert.Equal(t, "test-owner", requests[0].Owner)
	assert.NotEmpty(t, requests[0].OperationID)
	assert.Empty(t, d.coord.HeldLocks(), "the delegate lock is released after the operation")
}

func TestLockDelegation_ResourceIsScopedByTheTargetCollection(t *testing.T) {
	d := newDelegatingEnv(t, func(r *artifacttest.Repository) artifact.Repository {
		return &namedRepository{Repository: r, namespace: "/data/artifacts"}
	}, bothScopes...)

	_, err := d.svc.Publish(context.Background(), d.publishRequest("work", d.source("hello")))
	require.NoError(t, err)
	requests := d.coord.recorded()
	require.Len(t, requests, 1)
	assert.Equal(t, "/data/artifacts/tools/example:1.0.0", requests[0].Resource, "two aliases for one collection share a lock; unrelated collections do not")
}

func TestLockDelegation_CoversEveryMutatingOperation(t *testing.T) {
	d := newDelegatingEnv(t, nil, bothScopes...)
	published := d.publish("work", d.source("hello"))
	base := len(d.coord.recorded())

	signed, err := d.svc.Sign(context.Background(), artifact.SignRequest{Repository: "work", Ref: artifact.Ref{Name: "tools/example", Digest: published.Operation.RecordDigest}})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, signed.Operation.LockScope)

	deleted, err := d.svc.Delete(context.Background(), artifact.DeleteRequest{Repository: "work", Ref: exampleRef})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, deleted.LockScope)

	assert.Len(t, d.coord.recorded(), base+2, "sign and delete each lock through the delegate")
	assert.Zero(t, d.work.Calls("Lock"))
	assert.Empty(t, d.coord.HeldLocks())
}

func TestLockDelegation_ReleasesTheDelegateLockWhenThePublicationFails(t *testing.T) {
	d := newDelegatingEnv(t, nil, bothScopes...)
	d.work.InjectFailure(artifacttest.FailBeforePayload, errTransport, 10)

	_, err := d.svc.Publish(context.Background(), d.publishRequest("work", d.source("hello")))
	require.ErrorIs(t, err, errTransport)
	assert.NotEmpty(t, d.coord.recorded())
	assert.Empty(t, d.coord.HeldLocks(), "the delegate lock is released on the failure path")
}

func TestLockDelegation_CopyLocksTheDestinationDelegate(t *testing.T) {
	d := newDelegatingEnv(t, nil, bothScopes...)
	d.static.Repositories["src"] = artifacttest.New(artifacttest.Options{Alias: "src"})
	_, err := d.svc.Publish(context.Background(), d.publishRequest("src", d.source("hello")))
	require.NoError(t, err)

	result, err := d.svc.Copy(context.Background(), artifact.CopyRequest{Ref: exampleRef, From: "src", To: "work"})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, result.LockScope)
	assert.Zero(t, d.work.Calls("Lock"))
	assert.NotEmpty(t, d.coord.recorded())
	assert.Empty(t, d.coord.HeldLocks())
}

func TestLockDelegation_DelegateWithoutTheRequiredScopeFailsBeforeMutating(t *testing.T) {
	// The delegate only offers host scope, although the target's description claims repository scope.
	d := newDelegatingEnv(t, nil, artifact.LockScopeHost)
	caps := artifacttest.DefaultCapabilities()
	caps.LockScopes = bothScopes
	d.static.Info["work"] = service.RepositoryInfo{Capabilities: caps, Packaging: artifact.PackagingNone, RequiredLockScope: artifact.LockScopeRepository, LockProvider: "coord"}

	_, err := d.svc.Publish(context.Background(), d.publishRequest("work", d.source("hello")))
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	assert.Empty(t, d.coord.recorded(), "an insufficient delegate is rejected before any lock is taken")
	_, found := d.work.RecordDigestOf(exampleRef)
	assert.False(t, found, "nothing was published")
}

func TestLockDelegation_UnknownDelegateFailsBeforeMutating(t *testing.T) {
	d := newDelegatingEnv(t, nil, bothScopes...)
	delete(d.static.Repositories, "coord")

	_, err := d.svc.Publish(context.Background(), d.publishRequest("work", d.source("hello")))
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, found := d.work.RecordDigestOf(exampleRef)
	assert.False(t, found, "nothing was published")
}

func TestLockDelegation_ARepositoryThatLocksItselfIsUnchanged(t *testing.T) {
	e := newEnv(t, envConfig{})
	result, err := e.svc.Publish(context.Background(), e.publishRequest("dst", e.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeHost, result.LockScope)
	assert.NotZero(t, e.repos["dst"].Calls("Lock"))
}
