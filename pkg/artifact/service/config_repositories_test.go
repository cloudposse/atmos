package service_test

import (
	"context"
	"sync/atomic"
	"testing"

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
	assert.True(t, info.Capabilities.HasLockScope(artifact.LockScopeRepository), "the delegate's scope is advertised")
	assert.Equal(t, "default description", info.Defaults.Description)
	assert.Equal(t, map[string]string{"team": "platform"}, info.Defaults.Labels)
	assert.True(t, info.Policy.RequireSignature)
	assert.Len(t, info.Policy.TrustSets, 2, "global and repository trust sets are separate, both required")

	coordination, err := repos.Describe("coordination")
	require.NoError(t, err)
	assert.Equal(t, artifact.PackagingTgz, coordination.Packaging)
	assert.Equal(t, artifact.LockScopeHost, coordination.RequiredLockScope)
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
