package service_test

import (
	"context"
	"sync"
	"sync/atomic"
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

func configRegistry(t *testing.T, constructed *atomic.Int64) *artifact.RepositoryRegistry {
	t.Helper()
	registry := artifact.NewRepositoryRegistry()
	for _, kind := range []string{artifact.RepositoryKindLocalDir, artifact.RepositoryKindS3} {
		desc, ok := artifact.BuiltinKind(kind)
		require.True(t, ok)
		require.NoError(t, registry.Register(desc, func(_ context.Context, alias string, _ *schema.ArtifactRepositoryConfig, _ ...artifact.RepositoryOption) (artifact.Repository, error) {
			constructed.Add(1)
			return artifacttest.New(artifacttest.Options{Alias: alias}), nil
		}))
	}
	return registry
}

func boolPtr(b bool) *bool { return &b }

func TestConfigRepositories_DescribeIsOfflineAndCombinesPolicyDefaultsAndLocking(t *testing.T) {
	var constructed atomic.Int64
	cfg := &schema.ArtifactConfig{
		Verification: &schema.ArtifactVerificationConfig{
			RequireSignature:  boolPtr(true),
			TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: "global@example.com", Issuer: "https://global.example.com"}},
		},
		Repositories: map[string]schema.ArtifactRepositoryConfig{
			"shared": {
				Kind:    artifact.RepositoryKindLocalDir,
				Options: map[string]any{"path": "/tmp/shared"},
				Defaults: &schema.ArtifactRepositoryDefaults{Metadata: &schema.ArtifactMetadataDefaults{
					Description: "default description", Labels: map[string]string{"team": "platform"},
				}},
				Verification: &schema.ArtifactVerificationConfig{TrustedIdentities: []schema.ArtifactTrustedIdentity{{Identity: "repo@example.com", Issuer: "https://repo.example.com"}}},
				Locking:      &schema.ArtifactLockingConfig{RequiredScope: "repository", Repository: "coordination"},
			},
			"coordination": {Kind: artifact.RepositoryKindS3, Options: map[string]any{"bucket": "acme-coordination"}},
		},
	}
	repos := service.FromConfig(cfg, configRegistry(t, &constructed))

	assert.Equal(t, []string{"coordination", "shared"}, repos.Aliases())
	info, err := repos.Describe("shared")
	require.NoError(t, err)
	assert.Zero(t, constructed.Load(), "describing never constructs a repository, so it never authenticates")
	assert.Equal(t, artifact.PackagingNone, info.Packaging)
	assert.Equal(t, artifact.LockScopeRepository, info.RequiredLockScope)
	assert.Equal(t, "coordination", info.LockProvider, "the service routes locks to the delegate")
	assert.True(t, info.Capabilities.HasLockScope(artifact.LockScopeRepository), "the delegate's scope is advertised")
	assert.Equal(t, "default description", info.Defaults.Description)
	assert.Equal(t, map[string]string{"team": "platform"}, info.Defaults.Labels)
	assert.True(t, info.Policy.RequireSignature)
	assert.Len(t, info.Policy.TrustSets, 2, "global and repository trust sets are separate, both required")

	coordination, err := repos.Describe("coordination")
	require.NoError(t, err)
	assert.Equal(t, artifact.PackagingTgz, coordination.Packaging)
	assert.Equal(t, artifact.LockScopeHost, coordination.RequiredLockScope)
	assert.Empty(t, coordination.LockProvider, "a repository that locks itself names no provider")
	assert.Len(t, coordination.Policy.TrustSets, 1)
}

func TestConfigRepositories_OpenConstructsOnceAndRejectsUnknowns(t *testing.T) {
	var constructed atomic.Int64
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": "/tmp/shared"}},
		"ghost":  {Kind: "unregistered/kind"},
		"s3":     {Kind: artifact.RepositoryKindS3, Options: map[string]any{"bucket": "acme-artifacts"}},
	}}
	repos := service.FromConfig(cfg, configRegistry(t, &constructed))

	first, err := repos.Open(context.Background(), "shared")
	require.NoError(t, err)
	second, err := repos.Open(context.Background(), "shared")
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Equal(t, int64(1), constructed.Load())

	_, err = repos.Open(context.Background(), "missing")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = repos.Describe("missing")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = repos.Describe("ghost")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = repos.Open(context.Background(), "ghost")
	require.Error(t, err)
}

func TestConfigRepositories_DrivesTheServiceEndToEnd(t *testing.T) {
	var constructed atomic.Int64
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": "/tmp/shared"}},
	}}
	e := newEnv(t, envConfig{})
	svc, err := service.New(service.FromConfig(cfg, configRegistry(t, &constructed)), nil, service.WithStagingRoot(e.staging))
	require.NoError(t, err)

	dry := e.publishRequest("shared", e.source("hello"))
	dry.DryRun = true
	_, err = svc.Publish(context.Background(), dry)
	require.NoError(t, err)
	assert.Zero(t, constructed.Load(), "a dry run through configuration performs no repository construction or identity resolution")

	result, err := svc.Publish(context.Background(), e.publishRequest("shared", e.source("hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	assert.Equal(t, int64(1), constructed.Load())
}

// TestConfigRepositories_DescribeReturnsIndependentCopies proves a caller editing what Describe
// returned cannot change the registry's capabilities or the configured defaults, which every later
// Describe would otherwise serve.
func TestConfigRepositories_DescribeReturnsIndependentCopies(t *testing.T) {
	var constructed atomic.Int64
	labels := map[string]string{"team": "platform"}
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared": {
			Kind:     artifact.RepositoryKindLocalDir,
			Options:  map[string]any{"path": "/tmp/shared"},
			Defaults: &schema.ArtifactRepositoryDefaults{Metadata: &schema.ArtifactMetadataDefaults{Labels: labels}},
		},
	}}
	repos := service.FromConfig(cfg, configRegistry(t, &constructed))
	pristine, err := repos.Describe("shared")
	require.NoError(t, err)
	wantPackagings := append([]artifact.Packaging(nil), pristine.Capabilities.Packagings...)
	wantLockScopes := append([]artifact.LockScope(nil), pristine.Capabilities.LockScopes...)
	require.NotEmpty(t, wantPackagings)
	require.NotEmpty(t, wantLockScopes)

	first, err := repos.Describe("shared")
	require.NoError(t, err)
	first.Capabilities.Packagings[0] = artifact.PackagingZip
	first.Capabilities.LockScopes[0] = artifact.LockScopeRepository
	first.Defaults.Labels["team"] = "scribble"

	second, err := repos.Describe("shared")
	require.NoError(t, err)
	assert.Equal(t, wantPackagings, second.Capabilities.Packagings)
	assert.Equal(t, wantLockScopes, second.Capabilities.LockScopes)
	assert.Equal(t, map[string]string{"team": "platform"}, second.Defaults.Labels)
	assert.Equal(t, map[string]string{"team": "platform"}, labels, "the configured defaults are untouched")
}

// blockingRegistry builds repositories like configRegistry, but the construction of "slow" waits for
// release, so a test can hold one alias mid-construction.
func blockingRegistry(t *testing.T, started chan<- struct{}, release <-chan struct{}, constructed *atomic.Int64) *artifact.RepositoryRegistry {
	t.Helper()
	registry := artifact.NewRepositoryRegistry()
	desc, ok := artifact.BuiltinKind(artifact.RepositoryKindLocalDir)
	require.True(t, ok)
	require.NoError(t, registry.Register(desc, func(_ context.Context, alias string, _ *schema.ArtifactRepositoryConfig, _ ...artifact.RepositoryOption) (artifact.Repository, error) {
		constructed.Add(1)
		if alias == "slow" {
			started <- struct{}{}
			<-release
		}
		return artifacttest.New(artifacttest.Options{Alias: alias}), nil
	}))
	return registry
}

func blockingConfig() *schema.ArtifactConfig {
	return &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"fast": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": "/tmp/fast"}},
		"slow": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": "/tmp/slow"}},
	}}
}

// While one repository is being constructed (it may be resolving credentials), opening an already
// cached repository must not wait for it.
func TestConfigRepositories_OpenOfACachedRepositoryIsNotBlockedByAnotherConstruction(t *testing.T) {
	var constructed atomic.Int64
	started, release := make(chan struct{}, 1), make(chan struct{})
	repos := service.FromConfig(blockingConfig(), blockingRegistry(t, started, release, &constructed))
	cached, err := repos.Open(context.Background(), "fast")
	require.NoError(t, err)

	slowDone := make(chan error, 1)
	go func() {
		_, openErr := repos.Open(context.Background(), "slow")
		slowDone <- openErr
	}()
	<-started

	again := make(chan artifact.Repository, 1)
	go func() {
		repo, _ := repos.Open(context.Background(), "fast")
		again <- repo
	}()
	select {
	case repo := <-again:
		assert.Same(t, cached, repo)
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("opening a cached repository waited for another repository's construction")
	}
	close(release)
	require.NoError(t, <-slowDone)
}

// A caller waiting for a repository that another caller is constructing stops waiting when its own
// context ends, and the construction still completes for everyone else.
func TestConfigRepositories_AWaitingOpenHonorsItsContext(t *testing.T) {
	var constructed atomic.Int64
	started, release := make(chan struct{}, 1), make(chan struct{})
	repos := service.FromConfig(blockingConfig(), blockingRegistry(t, started, release, &constructed))

	firstDone := make(chan artifact.Repository, 1)
	go func() {
		repo, _ := repos.Open(context.Background(), "slow")
		firstDone <- repo
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	go func() {
		_, err := repos.Open(ctx, "slow")
		waiter <- err
	}()
	cancel()
	select {
	case err := <-waiter:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("a canceled waiter kept waiting for the construction")
	}
	close(release)
	require.NotNil(t, <-firstDone)
	assert.Equal(t, int64(1), constructed.Load(), "the waiter did not start a second construction")
}

// Callers that race for the same alias share one construction and one repository.
func TestConfigRepositories_ConcurrentOpensOfOneAliasConstructOnce(t *testing.T) {
	var constructed atomic.Int64
	started, release := make(chan struct{}, 8), make(chan struct{})
	repos := service.FromConfig(blockingConfig(), blockingRegistry(t, started, release, &constructed))

	const callers = 8
	results := make([]artifact.Repository, callers)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = repos.Open(context.Background(), "slow")
		}()
	}
	<-started
	close(release)
	wg.Wait()
	for _, repo := range results {
		require.NotNil(t, repo)
		assert.Same(t, results[0], repo)
	}
	assert.Equal(t, int64(1), constructed.Load())
}
