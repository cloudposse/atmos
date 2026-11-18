package discovery_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/catalog"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/internal/testkit"
	"github.com/cloudposse/atmos/pkg/artifact/service"
)

// catalogEnv is a registry that cannot enumerate repositories, with a catalog configured on it.
func catalogEnv(t *testing.T) *env {
	t.Helper()
	registry := testkit.NewRegistry(t)
	return newEnv(t, envOptions{noCatalog: true, catalog: registry.CatalogURI("catalog", "stable"), registryOverride: registry})
}

func (e *env) catalogs() *catalog.Service {
	return catalog.NewService(
		&service.StaticRepositories{Repositories: map[string]artifact.Repository{"shared": e.writerFor()}, Info: map[string]service.RepositoryInfo{"shared": e.info}},
		catalog.WithClock(func() time.Time { return epoch }),
	)
}

// writerFor returns a catalog-capable repository over the same registry.
func (e *env) writerFor() artifact.Repository { return e.writer }

func (e *env) addToCatalog(refs ...string) {
	e.t.Helper()
	parsed := make([]artifact.Ref, 0, len(refs))
	for _, text := range refs {
		ref, err := artifact.ParseRef(text)
		require.NoError(e.t, err)
		parsed = append(parsed, ref)
	}
	_, err := e.catalogs().Add(context.Background(), &catalog.AddRequest{Repository: "shared", Refs: parsed})
	require.NoError(e.t, err)
}

func TestCollectionIsListedFromTheConfiguredCatalogWhenTheRegistryCannotEnumerate(t *testing.T) {
	e := catalogEnv(t)
	vpc := e.publish("components/vpc", "1.0.0", map[string]string{"team": "platform"})
	e.publish("components/vpc", "1.1.0", nil)
	e.publish("unlisted/app", "1.0.0", nil)
	e.addToCatalog("components/vpc:1.0.0", "components/vpc:1.1.0")
	e.rebuild(envOptions{})
	e.registry.Reset()

	result, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|components/vpc:1.0.0", "shared|components/vpc:1.1.0"}, refs(result),
		"only catalog entries are listed; an artifact published since the catalog was updated is not")
	assert.Equal(t, "catalog", result.Repositories[0].Source)
	require.NotEmpty(t, result.Repositories[0].Diagnostics)
	assert.Equal(t, "catalog", result.Repositories[0].Diagnostics[0].Code)
	assert.Equal(t, vpc.Pinned.ResolvedDigest, result.Artifacts[0].Pinned.ResolvedDigest)
	assert.Equal(t, vpc.Pinned.RecordDigest, result.Artifacts[0].Pinned.RecordDigest)
	assert.Equal(t, map[string]string{"team": "platform"}, result.Artifacts[0].Labels)
	assert.Zero(t, e.registry.Count("GET", "/_catalog"), "the registry's repository catalog is not used")
	for _, request := range e.registry.Requests() {
		assert.NotContains(t, request, "/components/vpc/", "catalog discovery reads the catalog, never the listed artifacts: %s", request)
	}

	// Search and label filters apply to the catalog's published-version metadata.
	labeled, err := e.list(artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "platform"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|components/vpc:1.0.0"}, refs(labeled))

	// A fresh snapshot answers without any request, and named listing still reads the tags.
	e.registry.Reset()
	cached, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, refs(result), refs(cached))
	assert.Zero(t, e.registry.Total())
	named, err := e.list(artifact.ListRequest{Name: "unlisted/app"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|unlisted/app:1.0.0"}, refs(named), "explicit-name listing needs neither a catalog nor registry enumeration")
}

func TestCatalogRefreshReusesEntriesWhenTheCatalogDigestIsUnchanged(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	e.addToCatalog("app:1.0.0")
	e.rebuild(envOptions{})
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)

	e.rebuild(envOptions{})
	e.registry.Reset()
	again, err := e.list(artifact.ListRequest{Refresh: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0"}, refs(again))
	assert.Zero(t, e.registry.Count("GET", "/blobs/"), "an unchanged catalog digest reuses the snapshot without reading the payload: %v", e.registry.Requests())

	e.publish("app", "1.1.0", nil)
	e.addToCatalog("app:1.1.0")
	e.rebuild(envOptions{})
	changed, err := e.list(artifact.ListRequest{Refresh: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0", "shared|app:1.1.0"}, refs(changed))
}

func TestMissingConfiguredCatalogIsAnErrorNotAnEmptyListing(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	result, err := e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
	assert.Contains(t, err.Error(), "catalog")
	assert.Empty(t, result.Artifacts)
	assert.True(t, result.Incomplete)
}

func TestCatalogEntriesOutsideTheDestinationRepositoryAreRejectedAndNeverCached(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	e.addToCatalog("app:1.0.0")
	loaded, err := catalog.Read(context.Background(), e.writer, artifact.Ref{Name: "catalog", Version: "stable"})
	require.NoError(t, err)

	// A catalog that points an entry at another registry is data from an untrusted publisher.
	hostile := loaded.File
	hostile.Spec.Entries[0].Source = "oci://evil.example.com/acme/atmos/app"
	hostile.Metadata.Revision++
	_, err = catalog.Write(context.Background(), e.writer, artifact.Ref{Name: "catalog", Version: "stable"}, hostile, &catalog.WriteOptions{Prior: &loaded.Pinned, Now: epoch, OperationID: "op"})
	require.NoError(t, err)

	e.rebuild(envOptions{})
	e.registry.Reset()
	result, err := e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
	assert.Empty(t, result.Artifacts)
	assert.Empty(t, e.snapshots(), "a rejected catalog is not cached")
	for _, request := range e.registry.Requests() {
		assert.NotContains(t, request, "evil.example.com", "no request follows a location named by the catalog")
	}
}

func TestTamperedCatalogSnapshotIsRejectedByPolicyOnRead(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	e.addToCatalog("app:1.0.0")
	e.rebuild(envOptions{})
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)

	files := e.snapshots()
	require.Len(t, files, 1)
	raw, err := os.ReadFile(files[0])
	require.NoError(t, err)
	var snapshot map[string]any
	require.NoError(t, json.Unmarshal(raw, &snapshot))
	entries := snapshot["entries"].([]any)
	pinned := entries[0].(map[string]any)["pinned"].(map[string]any)
	pinned["repository"] = "evil.example.com/acme/atmos/app"
	tampered, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(files[0], tampered, 0o600))

	e.registry.Reset()
	_, err = e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid, "the location binding is re-checked when a snapshot is read")
	assert.Zero(t, e.registry.Total())
	e.offline.Store(true)
	_, err = e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactCatalogInvalid)
}

func TestCatalogReconfigurationStartsANewSnapshot(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	e.addToCatalog("app:1.0.0")
	e.rebuild(envOptions{})
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)

	e.info.Discovery.Catalog = e.registry.CatalogURI("other", "stable")
	e.rebuild(envOptions{})
	e.offline.Store(true)
	_, err = e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline, "a snapshot of one catalog is never served for another")
}

func TestDigestOnlyCatalogReferenceListsFromThatRevision(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	e.addToCatalog("app:1.0.0")
	pinned, err := e.writer.Resolve(context.Background(), artifact.Ref{Name: "catalog", Version: "stable"})
	require.NoError(t, err)
	e.publish("app", "1.1.0", nil)
	e.addToCatalog("app:1.1.0")

	e.info.Discovery.Catalog = "oci://" + e.registry.Location() + "/catalog@" + pinned.ResolvedDigest.String()
	e.rebuild(envOptions{})
	result, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0"}, refs(result), "a digest-pinned catalog is an immutable revision")
}

func TestCatalogDiagnosticNamesTheCatalogAndItsDigest(t *testing.T) {
	e := catalogEnv(t)
	e.publish("app", "1.0.0", nil)
	e.addToCatalog("app:1.0.0")
	e.rebuild(envOptions{})
	result, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	message := result.Repositories[0].Diagnostics[0].Message
	assert.Contains(t, message, e.info.Discovery.Catalog)
	assert.Contains(t, message, "sha256:")
	assert.True(t, strings.Contains(message, "not included"), message)
}
