package compose_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/artifact/s3"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
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

// The coordinator is opened from configuration with its own identity, rather than the target's.
func TestDelegatedMutationResolvesCoordinatorIdentity(t *testing.T) {
	fake := s3test.NewServer(t)
	creds := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.WriteFile(creds, []byte("[default]\naws_access_key_id = test\naws_secret_access_key = test\n"), 0o600))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	repos := map[string]schema.ArtifactRepositoryConfig{
		"work": {
			Kind: artifact.RepositoryKindLocalDir, Identity: "target-identity", Options: map[string]any{"path": t.TempDir()},
			Locking: &schema.ArtifactLockingConfig{RequiredScope: "repository", Repository: "coord"},
		},
		"coord": {
			Kind: artifact.RepositoryKindS3, Identity: "coordinator-identity",
			Options: map[string]any{"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1", "path_style": true},
		},
	}
	var mu sync.Mutex
	var identities []string
	resolver := func(_ context.Context, identity string) (any, error) {
		mu.Lock()
		identities = append(identities, identity)
		mu.Unlock()
		return &store.AWSAuthConfig{CredentialsFile: creds, Profile: "default", Region: "us-east-1"}, nil
	}
	composition := build(t, repos, nil, compose.WithIdentityResolver(resolver))
	_, err := composition.Repositories.Open(context.Background(), "work")
	require.NoError(t, err)
	_, err = composition.Repositories.Open(context.Background(), "coord")
	require.NoError(t, err)
	assert.Empty(t, identities, "opening repositories must not authenticate")
	_, err = composition.Service.Publish(context.Background(), publishRequest("work", source(t, "hello")))
	require.NoError(t, err)
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, identities)
	for _, identity := range identities {
		assert.Equal(t, "coordinator-identity", identity)
	}
}

func TestDelegatedS3MutationsUseTheirOwnReferenceClaim(t *testing.T) {
	bucket := newS3Fixture(t)
	trust := newTrustFixture(t)
	work := bucket.repository()
	work.Options["prefix"] = "work"
	work.Locking = &schema.ArtifactLockingConfig{RequiredScope: "repository", Repository: "coord"}
	coord := bucket.repository()
	coord.Options["prefix"] = "coord"
	composition := build(t, map[string]schema.ArtifactRepositoryConfig{"work": work, "coord": coord}, nil,
		trust.option, trust.signer, compose.WithS3Options(bucket.option))
	ctx := context.Background()
	published, err := composition.Service.Publish(ctx, publishRequest("work", source(t, "hello")))
	require.NoError(t, err)
	_, err = composition.Service.Sign(ctx, artifact.SignRequest{Repository: "work", Ref: artifact.Ref{Name: appName, Digest: published.RecordDigest}})
	require.NoError(t, err)
	_, err = composition.Service.Delete(ctx, artifact.DeleteRequest{Repository: "work", Ref: appRef()})
	require.NoError(t, err)
}
