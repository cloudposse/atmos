package s3

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/blob/s3/s3test"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newConfiguredService wires the local kinds and the S3 kind through the configuration-driven
// repository table, the way a composition root does. The S3 repositories run through the real SDK
// client against the fake endpoint.
func newConfiguredService(t *testing.T, fake *s3test.Server, extra map[string]schema.ArtifactRepositoryConfig) *service.Service {
	t.Helper()
	isolateAWS(t)
	root := t.TempDir()
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, local.RegisterWith(registry))
	require.NoError(t, RegisterWith(registry))
	repositories := map[string]schema.ArtifactRepositoryConfig{
		"files":  {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": filepath.Join(root, "files")}},
		"packed": {Kind: artifact.RepositoryKindLocalArchive, Packaging: "tgz", Options: map[string]any{"path": filepath.Join(root, "packed")}},
		"bucket-files": {
			Kind: artifact.RepositoryKindS3, Packaging: "none",
			Options: map[string]any{"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1", "prefix": "files", "tags": map[string]any{"team": "platform"}},
		},
		"bucket-archives": {
			Kind: artifact.RepositoryKindS3, Packaging: "tgz",
			Options: map[string]any{"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1", "prefix": "archives"},
		},
	}
	for alias := range extra {
		repositories[alias] = extra[alias]
	}
	svc, err := service.New(service.FromConfig(&schema.ArtifactConfig{Repositories: repositories}, registry), nil,
		service.WithStagingRoot(t.TempDir()), service.WithRetry(2, time.Millisecond),
		service.WithSleep(func(context.Context, time.Duration) error { return nil }))
	require.NoError(t, err)
	return svc
}

// An artifact moves between local and S3 repositories in both packagings, in both directions,
// without its Record identity changing.
func TestService_CopiesBetweenLocalAndS3(t *testing.T) {
	fake := s3test.NewServer(t)
	svc := newConfiguredService(t, fake, nil)
	ctx := context.Background()
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)

	published, err := svc.Publish(ctx, artifact.PublishRequest{Repository: "files", Source: src, Name: testName, Version: testVersion})
	require.NoError(t, err)
	require.Equal(t, artifact.OutcomeCreated, published.Outcome)

	hops := []struct{ from, to string }{
		{"files", "bucket-files"},
		{"bucket-files", "bucket-archives"},
		{"bucket-archives", "packed"},
		{"packed", "bucket-files"},
	}
	for index, hop := range hops {
		result, err := svc.Copy(ctx, artifact.CopyRequest{Ref: defaultRef(), From: hop.from, To: hop.to, Force: index == len(hops)-1})
		if index == len(hops)-1 {
			require.NoError(t, err, "%s to %s", hop.from, hop.to)
			assert.Equal(t, artifact.OutcomeUnchanged, result.Outcome, "copying the same Record again is a no-op")
			continue
		}
		require.NoError(t, err, "%s to %s", hop.from, hop.to)
		assert.Equal(t, artifact.OutcomeCreated, result.Outcome, "%s to %s", hop.from, hop.to)
	}

	listed, err := svc.List(ctx, artifact.ListRequest{})
	require.NoError(t, err)
	require.Len(t, listed.Artifacts, 4)
	for index := range listed.Artifacts {
		assert.Equal(t, published.RecordDigest, listed.Artifacts[index].Pinned.RecordDigest, "%s holds the same Record", listed.Artifacts[index].Repository)
	}

	for _, alias := range []string{"bucket-files", "bucket-archives"} {
		out := filepath.Join(t.TempDir(), alias)
		_, err = svc.Pull(ctx, artifact.PullRequest{Repository: alias, Ref: defaultRef(), Destination: out})
		require.NoError(t, err, alias)
		inspection, err := svc.Inspect(ctx, artifact.InspectRequest{Repository: alias, Ref: defaultRef()})
		require.NoError(t, err)
		state, err := materialize.InspectDestination(out, inspection.Inspection.Record)
		require.NoError(t, err)
		assert.Equal(t, materialize.DestinationMatches, state, alias)
		document, err := svc.ShowDocument(ctx, artifact.ShowDocumentRequest{Repository: alias, Ref: defaultRef(), Role: artifact.DocumentReadme})
		require.NoError(t, err)
		assert.Equal(t, fixtures.PackageReadme, string(document.Content))
	}

	pin, err := svc.Pin(ctx, artifact.PinRequest{Repository: "bucket-archives", Ref: defaultRef()})
	require.NoError(t, err)
	assert.Equal(t, published.RecordDigest, pin.Pinned.RecordDigest, "the pin is the canonical Record digest")

	_, err = svc.Delete(ctx, artifact.DeleteRequest{Repository: "bucket-files", Ref: defaultRef()})
	require.NoError(t, err)
	listed, err = svc.List(ctx, artifact.ListRequest{})
	require.NoError(t, err)
	assert.Len(t, listed.Artifacts, 3)
}

// A repository that requires repository-scope coordination reports it, and the service's lock is
// the S3 claim. (Routing a delegating repository's lock to an S3 repository is the service's job;
// this repository only has to be a valid delegate: it advertises the repository scope.)
func TestService_UsesRepositoryScopeClaims(t *testing.T) {
	fake := s3test.NewServer(t)
	svc := newConfiguredService(t, fake, map[string]schema.ArtifactRepositoryConfig{
		"coordinated": {
			Kind: artifact.RepositoryKindS3, Packaging: "tgz", Locking: &schema.ArtifactLockingConfig{RequiredScope: "repository"},
			Options: map[string]any{"bucket": fake.Bucket(), "endpoint": fake.Endpoint(), "region": "us-east-1", "prefix": "coordinated"},
		},
	})
	src := t.TempDir()
	fixtures.WriteFilePackage(t, src)

	result, err := svc.Publish(context.Background(), artifact.PublishRequest{Repository: "coordinated", Source: src, Name: testName, Version: testVersion})
	require.NoError(t, err)
	assert.Equal(t, artifact.LockScopeRepository, result.LockScope)
	claimed := false
	for _, request := range fake.RequestsFor(s3test.OpPutObject) {
		claimed = claimed || (request.Header.Get("If-None-Match") == "*" && containsLockSegment(request.Key))
	}
	assert.True(t, claimed, "the service lock is an S3 owner claim")

	assert.Empty(t, lockObjects(fake), "and released")
}

func containsLockSegment(key string) bool { return strings.Contains(key, "/locks/") }

// The service reconciles a commit whose response was lost by reading the completed reference.
func TestService_ReconcilesALostResponseAfterCommit(t *testing.T) {
	fake := s3test.NewServer(t)
	svc := newConfiguredService(t, fake, nil)
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", "payload\n", 0o644)
	fake.Inject(s3test.Fault{Op: s3test.OpPutObject, KeyPrefix: "files/" + refKey(""), Skip: 1, Action: s3test.DropAfterCommit})

	result, err := svc.Publish(context.Background(), artifact.PublishRequest{Repository: "bucket-files", Source: src, Name: testName, Version: testVersion})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	pulled, err := svc.Pull(context.Background(), artifact.PullRequest{Repository: "bucket-files", Ref: defaultRef(), Destination: filepath.Join(t.TempDir(), "out")})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeRead, pulled.Outcome)
}
