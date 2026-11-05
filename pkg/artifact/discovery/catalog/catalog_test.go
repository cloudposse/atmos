package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/internal/testkit"
	"github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

var now = time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)

// env is one registry repository with a catalog service over it.
type env struct {
	t        *testing.T
	registry *testkit.Registry
	repo     *oci.Repository
	info     service.RepositoryInfo
	service  *catalog.Service
	uri      string
}

func newEnv(t *testing.T, configured bool) *env {
	t.Helper()
	e := &env{t: t, registry: testkit.NewRegistry(t)}
	if configured {
		e.uri = e.registry.CatalogURI("catalog", "stable")
	}
	e.repo = e.registry.Repo(t, "shared", e.uri)
	e.info = e.registry.Info(e.repo, e.uri)
	e.service = catalog.NewService(
		&service.StaticRepositories{Repositories: map[string]artifact.Repository{"shared": e.repo}, Info: map[string]service.RepositoryInfo{"shared": e.info}},
		catalog.WithClock(func() time.Time { return now }), catalog.WithOwner("test-owner"),
	)
	return e
}

func (e *env) publish(name, version string, labels map[string]string) *artifact.PublishResult {
	e.t.Helper()
	return testkit.Publish(e.t, e.repo, testkit.Spec{Name: name, Version: version, Content: name + version, Labels: labels})
}

func ref(text string) artifact.Ref {
	parsed, err := artifact.ParseRef(text)
	if err != nil {
		panic(err)
	}
	return parsed
}

func refsOf(texts ...string) []artifact.Ref {
	out := make([]artifact.Ref, 0, len(texts))
	for _, text := range texts {
		out = append(out, ref(text))
	}
	return out
}

// latest reads the published catalog straight from the repository.
func (e *env) latest() *catalog.Loaded {
	e.t.Helper()
	loaded, err := catalog.Read(context.Background(), e.repo, ref("catalog:stable"))
	require.NoError(e.t, err)
	return loaded
}

func listed(file *catalog.File) []string {
	var out []string
	for _, entry := range file.Spec.Entries {
		for _, version := range entry.Versions {
			out = append(out, entry.Name+":"+version.Version)
		}
	}
	return out
}

func TestBuildReadsMetadataOnlyAndNeverPublishes(t *testing.T) {
	e := newEnv(t, true)
	vpc := e.publish("components/vpc", "1.2.0", map[string]string{"team": "platform"})
	e.publish("tools/helper", "0.1.0", nil)
	e.registry.Reset()

	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("components/vpc:1.2.0", "tools/helper:0.1.0")})
	require.NoError(t, err)
	assert.Equal(t, []string{"components/vpc:1.2.0", "tools/helper:0.1.0"}, listed(built.File))
	assert.Zero(t, built.File.Metadata.Revision)
	assert.Empty(t, built.BaseDigest, "no catalog is published yet")
	assert.Equal(t, "catalog", built.File.Metadata.Name)

	entry := built.File.Spec.Entries[0]
	assert.Equal(t, "oci://"+e.registry.Location()+"/components/vpc", entry.Source, "the entry is bound to the repository path, not to credentials")
	assert.Equal(t, "files", entry.Kind)
	assert.Equal(t, "package components/vpc", entry.Description)
	version := entry.Versions[0]
	assert.Equal(t, vpc.Pinned.ResolvedDigest.String(), version.ResolvedDigest)
	assert.Equal(t, vpc.Pinned.RecordDigest.String(), version.RecordDigest)
	assert.Equal(t, vpc.Pinned.ContentDigest.String(), version.ContentDigest)
	assert.Equal(t, map[string]string{"team": "platform"}, version.Labels)
	require.Len(t, version.Documents, 1)
	assert.Equal(t, "readme", version.Documents[0].Role)

	for _, request := range e.registry.Requests() {
		assert.NotContains(t, request, vpc.Representation.Payload[0].Digest.Encoded(), "build never reads a payload layer: %s", request)
		assert.False(t, strings.HasPrefix(request, "PUT ") || strings.HasPrefix(request, "POST "), "build never writes: %s", request)
	}
	_, err = catalog.Read(context.Background(), e.repo, ref("catalog:stable"))
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "build does not publish")
}

func TestBuildRecordsTheCatalogItWasBasedOn(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	_, err := e.service.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	published := e.latest()

	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0"), Name: "custom"})
	require.NoError(t, err)
	assert.Equal(t, 1, built.File.Metadata.Revision)
	assert.Equal(t, published.Digest.String(), built.BaseDigest)
	assert.Equal(t, "custom", built.File.Metadata.Name)
}

func TestBuildWithoutCatalogConfigurationStillBuilds(t *testing.T) {
	e := newEnv(t, false)
	e.publish("app", "1.0.0", nil)
	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	assert.Equal(t, []string{"app:1.0.0"}, listed(built.File))
}

func TestBuildRejectsUnresolvableAndConflictingReferences(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	_, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:9.9.9")})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	_, err = e.service.Build(context.Background(), &catalog.BuildRequest{Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	_, err = e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "missing", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}

func TestPublishWritesAGenericCatalogRecord(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", map[string]string{"env": "prod"})
	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)

	result, err := e.service.Publish(context.Background(), &catalog.PublishRequest{Repository: "shared", File: built.File})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Revision)
	assert.Equal(t, 1, result.Listed)
	assert.Equal(t, e.uri, result.Catalog)
	assert.Equal(t, []string{"app:1.0.0"}, result.Added)
	assert.NotEmpty(t, result.Digest)
	assert.Empty(t, result.PreviousDigest)
	assert.Equal(t, artifact.LockScopeHost, result.LockScope, "the repository coordinator guards the write")

	// The catalog is a generic Record of kind catalog with a direct JSON payload layer.
	pinned, err := e.repo.Resolve(context.Background(), ref("catalog:stable"))
	require.NoError(t, err)
	inspection, err := e.repo.Inspect(context.Background(), pinned)
	require.NoError(t, err)
	require.NotNil(t, inspection.Record)
	assert.Equal(t, artifact.KindCatalog, inspection.Record.Kind)
	assert.Equal(t, artifact.PackagingCatalog, inspection.Representation.Packaging)
	require.Len(t, inspection.Representation.Payload, 1)
	assert.Equal(t, artifact.MediaTypeCatalogPayload, inspection.Representation.Payload[0].MediaType)

	reference, err := name.ParseReference(e.registry.Location() + "/catalog:stable")
	require.NoError(t, err)
	descriptor, err := remote.Get(reference)
	require.NoError(t, err)
	var manifest ocispec.Manifest
	require.NoError(t, json.Unmarshal(descriptor.Manifest, &manifest))
	assert.Equal(t, artifact.MediaTypeCatalog, manifest.ArtifactType)
	assert.Equal(t, artifact.MediaTypeConfig, manifest.Config.MediaType, "the shared Record config media type is retained")
	require.Len(t, manifest.Layers, 1)
	assert.Equal(t, artifact.MediaTypeCatalogPayload, manifest.Layers[0].MediaType)

	loaded := e.latest()
	assert.Equal(t, []string{"app:1.0.0"}, listed(loaded.File))
	assert.Equal(t, now.Format("2006-01-02T15:04:05Z"), loaded.File.Metadata.GeneratedAt)
	assert.Equal(t, 1, loaded.File.Metadata.Revision)
}

func TestMutatingCommandsFailBeforeAnyWriteWithoutACatalog(t *testing.T) {
	e := newEnv(t, false)
	e.publish("app", "1.0.0", nil)
	file := catalog.New("catalog")
	e.registry.Reset()
	ctx := context.Background()

	_, err := e.service.Publish(ctx, &catalog.PublishRequest{Repository: "shared", File: file})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogNotConfigured)
	_, err = e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogNotConfigured)
	_, err = e.service.Remove(ctx, &catalog.RemoveRequest{Repository: "shared", Ref: ref("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogNotConfigured)
	_, err = e.service.Sync(ctx, &catalog.SyncRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogNotConfigured)
	assert.Zero(t, e.registry.Total(), "configuration is checked before the registry is contacted: %v", e.registry.Requests())
}

func TestAddRemoveAndSyncListWithoutTouchingArtifacts(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	vpc := e.publish("components/vpc", "1.2.0", nil)
	e.publish("components/vpc", "1.10.0", nil)
	e.publish("components/vpc", "1.9.0", nil)
	e.publish("tools/helper", "0.1.0", nil)

	added, err := e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("components/vpc:1.2.0", "components/vpc:1.10.0", "components/vpc:1.9.0")})
	require.NoError(t, err)
	assert.Len(t, added.Added, 3)
	assert.Equal(t, []string{"components/vpc:1.10.0", "components/vpc:1.2.0", "components/vpc:1.9.0"}, listed(e.latest().File),
		"versions are opaque labels sorted lexically, not as SemVer")

	synced, err := e.service.Sync(ctx, &catalog.SyncRequest{Repository: "shared", Refs: refsOf("tools/helper:0.1.0")})
	require.NoError(t, err)
	assert.Equal(t, []string{"tools/helper:0.1.0"}, synced.Added)
	assert.Equal(t, 4, synced.Listed, "entries absent from the inventory are retained")
	assert.Equal(t, 2, synced.Revision)
	assert.Equal(t, added.Digest, synced.PreviousDigest)

	e.registry.Reset()
	removed, err := e.service.Remove(ctx, &catalog.RemoveRequest{Repository: "shared", Ref: ref("components/vpc:1.2.0")})
	require.NoError(t, err)
	assert.Equal(t, []string{"components/vpc:1.2.0"}, removed.Removed)
	assert.Equal(t, []string{"components/vpc:1.10.0", "components/vpc:1.9.0", "tools/helper:0.1.0"}, listed(e.latest().File))
	for _, request := range e.registry.Requests() {
		assert.False(t, strings.HasPrefix(request, "DELETE "), "remove only unlists; it deletes nothing: %s", request)
		assert.NotContains(t, request, "/acme/atmos/components/vpc/", "remove never touches the artifact's repository: %s", request)
	}
	pinned, err := e.repo.Resolve(ctx, ref("components/vpc:1.2.0"))
	require.NoError(t, err, "the unlisted artifact is still published")
	assert.Equal(t, vpc.Pinned.ResolvedDigest, pinned.ResolvedDigest, "its content is unchanged")

	_, err = e.service.Remove(ctx, &catalog.RemoveRequest{Repository: "shared", Ref: ref("components/vpc:1.2.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound, "a version that is not listed cannot be unlisted")
	_, err = e.service.Remove(ctx, &catalog.RemoveRequest{Repository: "shared", Ref: artifact.Ref{Name: "tools/helper", Digest: vpc.Pinned.ResolvedDigest}})
	require.ErrorIs(t, err, errUtils.ErrArtifactRefInvalid)

	_, err = e.service.Remove(ctx, &catalog.RemoveRequest{Repository: "shared", Ref: ref("tools/helper:0.1.0")})
	require.NoError(t, err)
	assert.NotContains(t, strings.Join(listed(e.latest().File), ","), "tools/helper", "removing the last version drops the entry")
}

func TestRemoveWithoutAPublishedCatalogReportsNotFound(t *testing.T) {
	e := newEnv(t, true)
	_, err := e.service.Remove(context.Background(), &catalog.RemoveRequest{Repository: "shared", Ref: ref("app:1.0.0")})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestSyncUpdatesAListingWhoseTagMoved(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	e.publish("app", "1.0.0", nil)
	_, err := e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	before := e.latest().File.Spec.Entries[0].Versions[0].ResolvedDigest

	moved, err := e.repo.Publish(ctx, testkit.Prepare(t, &testkit.Spec{Name: "app", Version: "1.0.0", Content: "changed"}), artifact.PublishOptions{Force: true, ExpectedPrior: &artifact.PinnedReference{ResolvedDigest: digest.Digest(before)}})
	require.NoError(t, err)
	result, err := e.service.Sync(ctx, &catalog.SyncRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	assert.Equal(t, []string{"app:1.0.0"}, result.Updated)
	assert.Equal(t, moved.Pinned.ResolvedDigest.String(), e.latest().File.Spec.Entries[0].Versions[0].ResolvedDigest)
}

func TestPublishRejectsStaleRevisionsAndDigests(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	e.publish("app", "1.0.0", nil)
	e.publish("app", "1.1.0", nil)

	t.Run("snapshot built before the first catalog existed", func(t *testing.T) {
		built, err := e.service.Build(ctx, &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
		require.NoError(t, err)
		_, err = e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.1.0")})
		require.NoError(t, err)
		_, err = e.service.Publish(ctx, &catalog.PublishRequest{Repository: "shared", File: built.File})
		require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict)
		assert.Equal(t, []string{"app:1.1.0"}, listed(e.latest().File), "the stale snapshot did not overwrite newer metadata")
	})
	t.Run("revision moved after the snapshot", func(t *testing.T) {
		built, err := e.service.Build(ctx, &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
		require.NoError(t, err)
		_, err = e.service.Remove(ctx, &catalog.RemoveRequest{Repository: "shared", Ref: ref("app:1.1.0")})
		require.NoError(t, err)
		_, err = e.service.Publish(ctx, &catalog.PublishRequest{Repository: "shared", File: built.File})
		require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict)
	})
	t.Run("expected digest", func(t *testing.T) {
		built, err := e.service.Build(ctx, &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
		require.NoError(t, err)
		_, err = e.service.Publish(ctx, &catalog.PublishRequest{Repository: "shared", File: built.File, ExpectedDigest: "sha256:" + strings.Repeat("0", 64)})
		require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict)
		fresh, err := e.service.Publish(ctx, &catalog.PublishRequest{Repository: "shared", File: built.File, ExpectedDigest: built.BaseDigest})
		require.NoError(t, err)
		assert.Equal(t, []string{"app:1.0.0"}, listed(e.latest().File))
		assert.Equal(t, built.File.Metadata.Revision+1, fresh.Revision)
	})
}

func TestPublishAgainstANewCatalogRejectsAnExpectedDigest(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	_, err = e.service.Publish(context.Background(), &catalog.PublishRequest{Repository: "shared", File: built.File, ExpectedDigest: "sha256:" + strings.Repeat("1", 64)})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict)
}

func TestWriteDetectsACatalogThatChangedAfterItWasRead(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	e.publish("app", "1.0.0", nil)
	e.publish("app", "1.1.0", nil)
	_, err := e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	stale := e.latest() // A writer reads the latest catalog...

	_, err = e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.1.0")}) // ...another writer publishes...
	require.NoError(t, err)

	next := stale.File
	next.Metadata.Revision++
	_, err = catalog.Write(ctx, e.repo, ref("catalog:stable"), next, &catalog.WriteOptions{Prior: &stale.Pinned, Now: now, OperationID: "op"})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogConflict, "...so the final prior-reference check rejects the stale write")
	assert.Equal(t, []string{"app:1.0.0", "app:1.1.0"}, listed(e.latest().File))
}

func TestSerializedWritersReadTheLatestCatalogInsideTheGuard(t *testing.T) {
	e := newEnv(t, true)
	const writers = 6
	for index := range writers {
		e.publish(fmt.Sprintf("app%d", index), "1.0.0", nil)
	}
	var group sync.WaitGroup
	errs := make(chan error, writers)
	for index := range writers {
		group.Go(func() {
			_, err := e.service.Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: refsOf(fmt.Sprintf("app%d:1.0.0", index))})
			errs <- err
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	loaded := e.latest()
	assert.Len(t, listed(loaded.File), writers, "no concurrent writer's entry was lost")
	assert.Equal(t, writers, loaded.File.Metadata.Revision)
}

func TestPublishingOrDeletingAnArtifactNeverEditsTheCatalog(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	e.publish("app", "1.0.0", nil)
	_, err := e.service.Add(ctx, &catalog.AddRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	before := e.latest()

	later := e.publish("app", "2.0.0", nil)
	assert.Equal(t, before.Digest, e.latest().Digest, "publishing a new version leaves the catalog unchanged")
	_, err = e.repo.Delete(ctx, later.Pinned, artifact.DeleteOptions{Scope: artifact.DeleteManifest})
	require.NoError(t, err)
	first, err := e.repo.Resolve(ctx, ref("app:1.0.0"))
	require.NoError(t, err)
	_, err = e.repo.Delete(ctx, first, artifact.DeleteOptions{Scope: artifact.DeleteManifest})
	require.NoError(t, err)
	after := e.latest()
	assert.Equal(t, before.Digest, after.Digest, "deleting an artifact leaves the catalog unchanged")
	assert.Equal(t, []string{"app:1.0.0"}, listed(after.File))
}

func TestPublishRejectsLocationsOutsideTheDestinationRepository(t *testing.T) {
	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	for source, label := range map[string]string{
		"oci://evil.example.com/acme/atmos/app":             "another registry",
		"oci://" + e.registry.Host + "/other/prefix/app":    "another prefix",
		"oci://" + e.registry.Location() + "/different":     "another name",
		"oci://user:pass@" + e.registry.Location() + "/app": "credentials in the source",
	} {
		t.Run(label, func(t *testing.T) {
			file := *built.File
			file.Spec.Entries = append([]catalog.Entry(nil), built.File.Spec.Entries...)
			file.Spec.Entries[0].Source = source
			e.registry.Reset()
			_, err := e.service.Publish(context.Background(), &catalog.PublishRequest{Repository: "shared", File: &file})
			require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
			assert.Zero(t, e.registry.Total(), "validation happens before any access")
		})
	}
}

func TestCatalogLocationMustLieInsideTheRepository(t *testing.T) {
	registry := testkit.NewRegistry(t)
	location := registry.Location()
	good, err := catalog.RefOf(location, "oci://"+strings.ToUpper(registry.Host)+"/"+testkit.Prefix+"/catalog:stable")
	require.NoError(t, err)
	assert.Equal(t, artifact.Ref{Name: "catalog", Version: "stable"}, good)
	byDigest, err := catalog.RefOf(location, "oci://"+location+"/team/catalog@sha256:"+strings.Repeat("a", 64))
	require.NoError(t, err)
	assert.Equal(t, "team/catalog", byDigest.Name)
	assert.True(t, byDigest.IsDigest())

	for uri, want := range map[string]string{
		"https://" + location + "/catalog:stable":              "oci://",
		"oci://other.example.com/acme/atmos/catalog:stable":    "outside the repository path",
		"oci://" + registry.Host + "/elsewhere/catalog:stable": "outside the repository path",
		"oci://" + location + "/catalog":                       "version",
		"oci://" + location + "/":                              "outside the repository path",
		"oci://user:secret@" + location + "/catalog:stable":    "outside the repository path",
	} {
		_, err := catalog.RefOf(location, uri)
		require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid, uri)
		assert.Contains(t, err.Error(), want, uri)
		assert.NotContains(t, err.Error(), "secret", "credentials are never echoed")
	}
}

func TestReadRejectsArtifactsThatAreNotCatalogs(t *testing.T) {
	e := newEnv(t, true)
	e.publish("catalog", "stable", nil)
	_, err := catalog.Read(context.Background(), e.repo, ref("catalog:stable"))
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid, "an ordinary package is not a catalog")
}

func TestFilesRoundTripInventoriesAndSnapshots(t *testing.T) {
	dir := t.TempDir()
	inventory := filepath.Join(dir, "inventory.yaml")
	require.NoError(t, os.WriteFile(inventory, []byte("apiVersion: atmos/v1\nkind: AtmosArtifactInventory\nmetadata:\n  name: releases\nspec:\n  refs:\n    - tools/helper:0.1.0\n    - components/vpc:1.2.0\n"), 0o600))
	refs, err := catalog.ReadInventoryFile(inventory)
	require.NoError(t, err)
	assert.Equal(t, []string{"components/vpc:1.2.0", "tools/helper:0.1.0"}, []string{refs[0].String(), refs[1].String()}, "inventory refs are sorted")

	_, err = catalog.ReadInventoryFile(filepath.Join(dir, "missing.yaml"))
	require.ErrorIs(t, err, os.ErrNotExist)

	e := newEnv(t, true)
	e.publish("app", "1.0.0", nil)
	built, err := e.service.Build(context.Background(), &catalog.BuildRequest{Repository: "shared", Refs: refsOf("app:1.0.0")})
	require.NoError(t, err)
	for _, name := range []string{"catalog.json", "catalog.yaml", "catalog.yml"} {
		path := filepath.Join(dir, name)
		require.NoError(t, catalog.WriteFile(path, built.File))
		read, err := catalog.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, built.File.Spec, read.Spec, name)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog.json"))
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))
	yamlRaw, err := os.ReadFile(filepath.Join(dir, "catalog.yaml"))
	require.NoError(t, err)
	assert.False(t, json.Valid(yamlRaw))
	assert.Contains(t, string(yamlRaw), "kind: AtmosArtifactCatalog")

	oversized := filepath.Join(dir, "huge.json")
	require.NoError(t, os.WriteFile(oversized, []byte(strings.Repeat(" ", int(artifact.MaxRecordSize)+1)), 0o600))
	_, err = catalog.ReadFile(oversized)
	require.ErrorIs(t, err, errUtils.ErrArtifactTooLarge)
}
