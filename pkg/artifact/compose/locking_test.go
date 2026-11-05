package compose_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/artifact/s3"
	"github.com/cloudposse/atmos/pkg/schema"
)

// A local (host-scope) repository that delegates locking to an S3 coordinator is coordinated with
// the coordinator's repository-scope claims, and the result reports that actual scope.
func TestLockDelegationEndToEndLocalThroughS3(t *testing.T) {
	coordinator := newS3Fixture(t)
	repos := map[string]schema.ArtifactRepositoryConfig{
		"work": {
			Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()},
			Locking: &schema.ArtifactLockingConfig{RequiredScope: "repository", Repository: "coord"},
		},
		"coord": coordinator.repository(),
	}
	composition := build(t, repos, nil, compose.WithS3Options(coordinator.option, s3.WithLockWait(150*time.Millisecond)))

	info, err := composition.Repositories.Describe("work")
	require.NoError(t, err)
	assert.Equal(t, "coord", info.LockProvider)
	assert.True(t, info.Capabilities.HasLockScope(artifact.LockScopeRepository), "the delegate's repository scope is advertised before any mutation")

	published, err := composition.Service.Publish(context.Background(), publishRequest("work", source(t, "hello")))
	require.NoError(t, err, "a host-scope local repository publishes once its locking is delegated")
	assert.Equal(t, artifact.OutcomeCreated, published.Outcome)
	assert.Equal(t, artifact.LockScopeRepository, published.LockScope, "the actual scope is the coordinator's")

	deleted, err := composition.Service.Delete(context.Background(), artifact.DeleteRequest{Repository: "work", Ref: appRef()})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, deleted.LockScope)
}

// The coordinator's claim really gates the target: while another writer holds the same resource,
// the mutation waits and then fails without writing, and it proceeds once the claim is released.
func TestLockDelegationBlocksOnAHeldClaimAndProceedsAfterRelease(t *testing.T) {
	coordinator := newS3Fixture(t)
	repos := map[string]schema.ArtifactRepositoryConfig{
		"work": {
			Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": t.TempDir()},
			Locking: &schema.ArtifactLockingConfig{RequiredScope: "repository", Repository: "coord"},
		},
		"coord": coordinator.repository(),
	}
	composition := build(t, repos, nil, compose.WithS3Options(coordinator.option, s3.WithLockWait(150*time.Millisecond)))
	ctx := context.Background()

	work, err := composition.Repositories.Open(ctx, "work")
	require.NoError(t, err)
	namespacer, ok := work.(artifact.LockNamespacer)
	require.True(t, ok, "a local repository names its collection for delegated lock keys")
	coord, err := composition.Repositories.Open(ctx, "coord")
	require.NoError(t, err)
	held, err := coord.Lock(ctx, artifact.LockRequest{Resource: namespacer.LockNamespace() + "/" + appName + ":" + appVersion, Owner: "other-writer", Required: artifact.LockScopeRepository})
	require.NoError(t, err)

	_, err = composition.Service.Publish(ctx, publishRequest("work", source(t, "hello")))
	require.Error(t, err, "another writer holds the claim for this exact reference")
	require.ErrorIs(t, err, errUtils.ErrArtifactConflict)
	_, resolveErr := work.Resolve(ctx, appRef())
	require.ErrorIs(t, resolveErr, errUtils.ErrArtifactNotFound, "nothing was written while the claim was held")

	require.NoError(t, held.Release(ctx))
	_, err = composition.Service.Publish(ctx, publishRequest("work", source(t, "hello")))
	require.NoError(t, err, "the same publication succeeds once the claim is gone")
}

func TestARepositoryWithoutDelegationKeepsHostScope(t *testing.T) {
	repos := map[string]schema.ArtifactRepositoryConfig{"dir": localRepo(artifact.RepositoryKindLocalDir, t.TempDir())}
	composition := build(t, repos, nil)

	published, err := composition.Service.Publish(context.Background(), publishRequest("dir", source(t, "hello")))
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeHost, published.LockScope)
}

func TestInsufficientLocalScopeIsRejectedBeforeAnyWrite(t *testing.T) {
	dir := t.TempDir()
	repos := map[string]schema.ArtifactRepositoryConfig{
		"dir": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": dir}, Locking: &schema.ArtifactLockingConfig{RequiredScope: "repository"}},
	}
	_, err := compose.New(&schema.AtmosConfiguration{Artifacts: schema.ArtifactConfig{Repositories: repos}})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "configuration requires a delegate that can provide the scope")
}
