package oci

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/artifacttest"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestHostGuardIsKeyedByRepositoryAndResourceAndNeverClaimsDistributedScope(t *testing.T) {
	ctx := context.Background()
	shared := t.TempDir()
	reg := newRegistry(t, nil)
	first := newRegistryRepo(t, reg, WithLockDir(shared))
	otherConfig := registryConfig(reg.server.Host)
	otherConfig.Options["prefix"] = "another/collection"
	second, err := NewRepository(ctx, "second", otherConfig, reg.options(WithLockDir(shared))...)
	require.NoError(t, err)

	request := artifact.LockRequest{Resource: "team/app:1.0.0", Owner: "a", OperationID: "op-a", Required: artifact.LockScopeHost}
	held, err := first.Lock(ctx, request)
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeHost, held.Scope(), "an OCI guard reports the host scope it really provides")
	_, expires := held.ExpiresAt()
	assert.False(t, expires)

	different := request
	different.Resource = "team/app:2.0.0"
	other, err := first.Lock(ctx, different)
	require.NoError(t, err, "a different tag does not contend")
	require.NoError(t, other.Release(ctx))

	elsewhere, err := second.Lock(ctx, request)
	require.NoError(t, err, "the same tag in another collection does not contend")
	require.NoError(t, elsewhere.Release(ctx))

	blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err = first.Lock(blocked, artifact.LockRequest{Resource: request.Resource, Owner: "b"})
	require.ErrorIs(t, err, context.DeadlineExceeded, "a competing writer waits and is bounded by its context")

	require.NoError(t, held.Release(ctx))
	require.Error(t, held.Release(ctx), "a released handle cannot release again")
	again, err := first.Lock(ctx, artifact.LockRequest{Resource: request.Resource, Owner: "b"})
	require.NoError(t, err, "the canceled acquisition leaked no ownership")
	require.NoError(t, again.Release(ctx))
}

func TestHostGuardRejectsWhatItCannotProvide(t *testing.T) {
	repo := newRegistryRepo(t, newRegistry(t, nil))
	ctx := context.Background()
	_, err := repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Required: artifact.LockScopeRepository})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a tag is not a distributed lease")
	assert.Contains(t, err.Error(), "locking.repository")
	_, err = repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Lease: time.Minute})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "a lease cannot be fenced")
	_, err = repo.Lock(ctx, artifact.LockRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)
}

// delegateEnv wires an OCI repository whose locks come from a configured coordinator.
type delegateEnv struct {
	repo        *Repository
	coordinator *artifacttest.Repository
}

func newDelegateEnv(t *testing.T, delegate string, scopes ...artifact.LockScope) *delegateEnv {
	t.Helper()
	caps := artifacttest.DefaultCapabilities()
	caps.LockScopes = scopes
	coordinator := artifacttest.New(artifacttest.Options{Alias: "coordination", Capabilities: &caps})
	lookup := func(alias string) (artifact.Repository, error) {
		if alias != "coordination" {
			return nil, errUtils.ErrArtifactConfigInvalid
		}
		return coordinator, nil
	}
	cfg := registryConfig("registry.example.com")
	cfg.Locking = &schema.ArtifactLockingConfig{RequiredScope: string(artifact.LockScopeRepository), Repository: delegate}
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry, WithTransport(newRegistry(t, nil).transport), WithLockDir(t.TempDir())))
	opened, err := registry.New(context.Background(), "shared", cfg, artifact.WithRepositoryLookup(lookup))
	require.NoError(t, err)
	repo, ok := opened.(*Repository)
	require.True(t, ok)
	return &delegateEnv{repo: repo, coordinator: coordinator}
}

func TestRepositoryScopeLockIsDelegatedToTheConfiguredCoordinator(t *testing.T) {
	ctx := context.Background()
	env := newDelegateEnv(t, "coordination", artifact.LockScopeHost, artifact.LockScopeRepository)
	handle, err := env.repo.Lock(ctx, artifact.LockRequest{Resource: "team/app:1.0.0", Owner: "writer", Required: artifact.LockScopeRepository})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, handle.Scope(), "the delegate's real scope is reported")
	held := env.coordinator.HeldLocks()
	require.Len(t, held, 1)
	assert.Contains(t, held[0], "registry.example.com/acme/atmos/team/app:1.0.0", "the coordinator key is scoped to this collection")

	require.NoError(t, handle.Release(ctx))
	assert.Empty(t, env.coordinator.HeldLocks())
}

func TestDelegationFailsClosed(t *testing.T) {
	ctx := context.Background()
	t.Run("delegate lacks the required scope", func(t *testing.T) {
		env := newDelegateEnv(t, "coordination", artifact.LockScopeHost)
		_, err := env.repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Required: artifact.LockScopeRepository})
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
		assert.Empty(t, env.coordinator.HeldLocks())
	})
	t.Run("unknown delegate", func(t *testing.T) {
		env := newDelegateEnv(t, "missing", artifact.LockScopeRepository)
		_, err := env.repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Required: artifact.LockScopeRepository})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("no lookup supplied", func(t *testing.T) {
		repo := newRegistryRepo(t, newRegistry(t, nil))
		repo.delegate = "coordination"
		_, err := repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Required: artifact.LockScopeRepository})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("self delegation", func(t *testing.T) {
		repo := newRegistryRepo(t, newRegistry(t, nil))
		repo.delegate = repo.alias
		repo.lookup = func(string) (artifact.Repository, error) { return repo, nil }
		_, err := repo.Lock(ctx, artifact.LockRequest{Resource: "x:1"})
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	})
	t.Run("a canceled wait on the delegate leaks nothing", func(t *testing.T) {
		env := newDelegateEnv(t, "coordination", artifact.LockScopeRepository)
		first, err := env.repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Owner: "a", Required: artifact.LockScopeRepository})
		require.NoError(t, err)
		canceled, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		_, err = env.repo.Lock(canceled, artifact.LockRequest{Resource: "x:1", Owner: "b", Required: artifact.LockScopeRepository})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NoError(t, first.Release(ctx))
		again, err := env.repo.Lock(ctx, artifact.LockRequest{Resource: "x:1", Owner: "b", Required: artifact.LockScopeRepository})
		require.NoError(t, err)
		require.NoError(t, again.Release(ctx))
	})
}

// Through the service, a repository that requires repository scope publishes under the delegate's
// lock and reports the scope that actually coordinated the write.
func TestServicePublishesUnderTheDelegatedLockAndReportsItsScope(t *testing.T) {
	ctx := context.Background()
	env := newDelegateEnv(t, "coordination", artifact.LockScopeHost, artifact.LockScopeRepository)
	layoutRepo := newLayoutRepo(t)
	layoutRepo.delegate, layoutRepo.lookup = "coordination", func(string) (artifact.Repository, error) { return env.coordinator, nil }
	caps := layoutRepo.Capabilities()
	caps.LockScopes = append(caps.LockScopes, artifact.LockScopeRepository)
	svc, err := service.New(&service.StaticRepositories{
		Repositories: map[string]artifact.Repository{"shared": layoutRepo},
		Info:         map[string]service.RepositoryInfo{"shared": {Capabilities: caps, RequiredLockScope: artifact.LockScopeRepository}},
	}, nil, service.WithStagingRoot(t.TempDir()))
	require.NoError(t, err)

	source := t.TempDir()
	writeSource(t, source)
	outcome, err := svc.PublishDetailed(ctx, artifact.PublishRequest{Repository: "shared", Source: source, Name: appName, Version: appVersion})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, outcome.Operation.LockScope)
	assert.Empty(t, env.coordinator.HeldLocks(), "the delegated lock was released")
}
