package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
	"github.com/cloudposse/atmos/pkg/artifact/materialize"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newConfiguredService wires the registered local kinds through the configuration-driven
// repository table, the way a composition root does.
func newConfiguredService(t *testing.T) (*service.Service, string) {
	t.Helper()
	root := t.TempDir()
	registry := artifact.NewRepositoryRegistry()
	require.NoError(t, RegisterWith(registry))
	// Windows cannot enforce POSIX file modes, so an explicit permissions request is refused there.
	permissions := "0600"
	if runtime.GOOS == "windows" {
		permissions = ""
	}
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"files": {Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": filepath.Join(root, "files")}},
		"packed": {
			Kind: artifact.RepositoryKindLocalArchive, Packaging: "tgz", Permissions: permissions,
			Options: map[string]any{"path": filepath.Join(root, "packed")},
		},
	}}
	svc, err := service.New(service.FromConfig(cfg, registry), nil,
		service.WithStagingRoot(t.TempDir()), service.WithRetry(2, time.Millisecond), service.WithSleep(func(context.Context, time.Duration) error { return nil }))
	require.NoError(t, err)
	return svc, root
}

func TestService_DrivesBothLocalBackends(t *testing.T) {
	svc, root := newConfiguredService(t)
	ctx := context.Background()
	src := t.TempDir()
	pkg := fixtures.WriteFilePackage(t, src)

	published, err := svc.Publish(ctx, artifact.PublishRequest{Repository: "files", Source: src, Name: testName, Version: testVersion})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, published.Outcome)
	assert.Equal(t, artifact.LockScopeHost, published.LockScope)
	assert.Equal(t, filepath.Join(root, "files", "team", "app", "1.0.0"), published.Location)

	again, err := svc.Publish(ctx, artifact.PublishRequest{Repository: "files", Source: src, Name: testName, Version: testVersion})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeUnchanged, again.Outcome, "an identical republication is a no-op")

	// Local directory to local archive: Record identity survives repackaging.
	copied, err := svc.Copy(ctx, artifact.CopyRequest{Ref: defaultRef(), From: "files", To: "packed"})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeCreated, copied.Outcome)
	assert.Equal(t, filepath.Join(root, "packed", "team", "app", "1.0.0.tgz"), copied.Location)

	listed, err := svc.List(ctx, artifact.ListRequest{})
	require.NoError(t, err)
	require.Len(t, listed.Artifacts, 2)
	assert.Equal(t, listed.Artifacts[0].Pinned.RecordDigest, listed.Artifacts[1].Pinned.RecordDigest, "the same Record in both packagings")

	document, err := svc.ShowDocument(ctx, artifact.ShowDocumentRequest{Repository: "packed", Ref: defaultRef(), Role: artifact.DocumentReadme})
	require.NoError(t, err)
	assert.Equal(t, fixtures.PackageReadme, string(document.Content))

	out := filepath.Join(t.TempDir(), "pulled")
	pulled, err := svc.Pull(ctx, artifact.PullRequest{Repository: "packed", Ref: defaultRef(), Destination: out})
	require.NoError(t, err)
	assert.Equal(t, artifact.OutcomeRead, pulled.Outcome)
	inspection, err := svc.Inspect(ctx, artifact.InspectRequest{Repository: "files", Ref: defaultRef()})
	require.NoError(t, err)
	state, err := materialize.InspectDestination(out, inspection.Inspection.Record)
	require.NoError(t, err)
	assert.Equal(t, materialize.DestinationMatches, state)
	assert.Equal(t, fixtures.FilePackagePaths(pkg.Symlinks), listInventoryPaths(inspection.Inspection.Record))

	pin, err := svc.Pin(ctx, artifact.PinRequest{Repository: "packed", Ref: defaultRef()})
	require.NoError(t, err)
	assert.Equal(t, inspection.Inspection.Pinned.RecordDigest, pin.Pinned.RecordDigest)

	_, err = svc.Delete(ctx, artifact.DeleteRequest{Repository: "files", Ref: defaultRef()})
	require.NoError(t, err)
	listed, err = svc.List(ctx, artifact.ListRequest{})
	require.NoError(t, err)
	require.Len(t, listed.Artifacts, 1)
	assert.Equal(t, "packed", listed.Artifacts[0].Repository)
	assert.NoDirExists(t, filepath.Join(root, "files", "team"))
}

// The service reconciles a commit whose response was lost by reading the completed reference.
func TestService_ReconcilesALostResponseAfterCommit(t *testing.T) {
	for _, spec := range []*repoSpec{dirSpec(), archiveSpec(artifact.PackagingTgz)} {
		t.Run(spec.kind, func(t *testing.T) {
			spec = spec.withOpts(withFaults(crashAt(faultAfterCommit)))
			repo := newRepo(t, spec)
			svc, err := service.New(&service.StaticRepositories{Repositories: map[string]artifact.Repository{"r": repo}}, nil, service.WithStagingRoot(t.TempDir()))
			require.NoError(t, err)
			src := t.TempDir()
			fixtures.WriteFile(t, src, "data.txt", "payload\n", 0o644)

			result, err := svc.Publish(context.Background(), artifact.PublishRequest{Repository: "r", Source: src, Name: testName, Version: testVersion})
			require.NoError(t, err, "the committed reference proves success")
			var reconciled bool
			for _, diagnostic := range result.Diagnostics {
				reconciled = reconciled || diagnostic.Code == "reconciled"
			}
			assert.True(t, reconciled, "success was confirmed by reading the reference: %v", result.Diagnostics)
			assertIntact(t, repo, defaultRef())
		})
	}
}

// An interruption before the commit is surfaced as unavailable, and the service's own retry of the
// same intent repairs it.
func TestService_RetryRepairsAnInterruptedPublication(t *testing.T) {
	var fault armedFault
	spec := dirSpec()
	spec = spec.withOpts(fault.option())
	repo := newRepo(t, spec)
	fault.hook = crashAt(faultAfterMetadata)
	svc, err := service.New(&service.StaticRepositories{Repositories: map[string]artifact.Repository{"r": repo}}, nil,
		service.WithStagingRoot(t.TempDir()), service.WithRetry(1, time.Millisecond))
	require.NoError(t, err)
	src := t.TempDir()
	fixtures.WriteFile(t, src, "data.txt", "payload\n", 0o644)
	request := artifact.PublishRequest{Repository: "r", Source: src, Name: testName, Version: testVersion}

	_, err = svc.Publish(context.Background(), request)
	require.Error(t, err)
	_, err = repo.Resolve(context.Background(), defaultRef())
	require.ErrorIs(t, err, errUtils.ErrArtifactPublicationUnavailable)

	result, err := svc.Publish(context.Background(), request)
	require.NoError(t, err, "a later invocation with the same intent repairs the pending operation")
	assert.Equal(t, artifact.OutcomeCreated, result.Outcome)
	assertIntact(t, repo, defaultRef())
	_, statErr := os.Stat(filepath.Join(repo.root, ".atmos-artifacts", "v1", "operations"))
	require.NoError(t, statErr)
	assert.Empty(t, listFiles(t, filepath.Join(repo.root, ".atmos-artifacts", "v1", "operations")))
}
