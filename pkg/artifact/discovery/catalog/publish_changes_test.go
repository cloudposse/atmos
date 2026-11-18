package catalog_test

import (
	"context"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/internal/testkit"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishReportsReplacementChanges(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	e.publish("app", "1.0.0", nil)
	e.publish("old", "1.0.0", nil)
	e.publish("new", "1.0.0", nil)
	_, err := e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0", "old:1.0.0")})
	require.NoError(t, err)
	var before string
	for _, entry := range e.latest().File.Spec.Entries {
		if entry.Name == "app" {
			before = entry.Versions[0].ResolvedDigest
		}
	}
	require.NotEmpty(t, before)
	_, err = e.repo.Publish(ctx, testkit.Prepare(t, &testkit.Spec{Name: "app", Version: "1.0.0", Content: "changed"}), artifact.PublishOptions{Force: true, ExpectedPrior: &artifact.PinnedReference{ResolvedDigest: digest.Digest(before)}})
	require.NoError(t, err)
	built, err := e.service.Build(ctx, &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0", "new:1.0.0")})
	require.NoError(t, err)
	result, err := e.service.Publish(ctx, &catalog.PublishRequest{Repository: "shared", File: built.File})
	require.NoError(t, err)
	assert.Equal(t, []string{"new:1.0.0"}, result.Added)
	assert.Equal(t, []string{"app:1.0.0"}, result.Updated)
	assert.Equal(t, []string{"old:1.0.0"}, result.Removed)
}

func TestBuildRejectsS3BeforeOpeningRepository(t *testing.T) {
	e := newEnv(t, false)
	info := service.RepositoryInfo{Discovery: service.DiscoveryInfo{Kind: artifact.RepositoryKindS3, Location: "s3://bucket/prefix"}}
	svc := catalog.NewService(&service.StaticRepositories{Repositories: map[string]artifact.Repository{"s3": e.repo}, Info: map[string]service.RepositoryInfo{"s3": info}})
	_, err := svc.Build(context.Background(), &catalog.BuildRequest{Repository: "s3", Refs: refsOf("app:1")})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
}
