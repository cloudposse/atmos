package discovery_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/discovery"
	"github.com/cloudposse/atmos/pkg/artifact/discovery/internal/testkit"
	"github.com/cloudposse/atmos/pkg/artifact/local"
	"github.com/cloudposse/atmos/pkg/artifact/oci"
	"github.com/cloudposse/atmos/pkg/artifact/service"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard for the schema fields these tests configure.
var _ = schema.ArtifactRepositoryConfig{Kind: "local/dir", Catalog: "", Identity: ""}

var epoch = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// clock is a settable time source shared by the cache and the test.
type clock struct{ now atomic.Pointer[time.Time] }

func newClock() *clock {
	c := &clock{}
	c.set(epoch)
	return c
}

func (c *clock) Now() time.Time          { return *c.now.Load() }
func (c *clock) set(at time.Time)        { c.now.Store(&at) }
func (c *clock) advance(d time.Duration) { c.set(c.Now().Add(d)) }

// countingRepositories counts how often each repository is opened, which is when credentials
// would be resolved.
type countingRepositories struct {
	*service.StaticRepositories
	opens atomic.Int64
}

func (c *countingRepositories) Open(ctx context.Context, alias string) (artifact.Repository, error) {
	c.opens.Add(1)
	return c.StaticRepositories.Open(ctx, alias)
}

// env wires one OCI registry repository, a discovery cache, and a service.
type env struct {
	t        *testing.T
	registry *testkit.Registry
	repo     *oci.Repository
	writer   *oci.Repository
	repos    *countingRepositories
	disc     *discovery.Discovery
	svc      *service.Service
	clock    *clock
	dir      string
	scope    string
	offline  atomic.Bool
	info     service.RepositoryInfo

	repoOptions []oci.Option
	catalog     string
}

// envOptions customize the environment.
type envOptions struct {
	catalog    string
	noCatalog  bool
	policy     discovery.Policy
	scope      string
	identity   string
	extraRepos map[string]artifact.Repository
	extraInfo  map[string]service.RepositoryInfo

	// registryOverride reuses a registry the caller already started (its catalog URI names it).
	registryOverride *testkit.Registry
}

//nolint:gocritic // Test helper; the options are a small literal.
func newEnv(t *testing.T, options envOptions) *env {
	t.Helper()
	registry := options.registryOverride
	if registry == nil {
		registry = testkit.NewRegistry(t)
	}
	e := &env{t: t, registry: registry, clock: newClock(), dir: filepath.Join(t.TempDir(), "discovery"), scope: options.scope}
	if e.scope == "" {
		e.scope = "scope-a"
	}
	var repoOptions []oci.Option
	if options.noCatalog {
		repoOptions = append(repoOptions, oci.WithoutCollectionEnumeration())
	}
	e.writer = e.registry.Repo(t, "writer", options.catalog)
	e.repoOptions = repoOptions
	e.repo = e.registry.Repo(t, "shared", options.catalog, repoOptions...)
	e.info = e.registry.Info(e.repo, options.catalog)
	e.info.Discovery.Identity = options.identity
	e.catalog = options.catalog
	e.rebuild(options)
	return e
}

// rebuild creates a fresh service and discovery over the same directory and registry, like a new
// process invocation.
//
//nolint:gocritic // Test helper; the options are a small literal.
func (e *env) rebuild(options envOptions) {
	e.repo = e.registry.Repo(e.t, "shared", e.catalog, e.repoOptions...)
	repositories := map[string]artifact.Repository{"shared": e.repo}
	infos := map[string]service.RepositoryInfo{"shared": e.info}
	for alias, repo := range options.extraRepos {
		repositories[alias] = repo
	}
	for alias := range options.extraInfo {
		infos[alias] = options.extraInfo[alias]
	}
	e.repos = &countingRepositories{StaticRepositories: &service.StaticRepositories{Repositories: repositories, Info: infos}}
	scope := e.scope
	discOptions := []discovery.Option{
		discovery.WithDirectory(e.dir), discovery.WithClock(e.clock.Now),
		discovery.WithOffline(func(*service.RepositoryInfo) bool { return e.offline.Load() }),
	}
	if options.policy != nil {
		discOptions = append(discOptions, discovery.WithPolicy(options.policy))
	}
	e.disc = discovery.New(func() (string, error) { return scope, nil }, discOptions...)
	svc, err := service.New(e.repos, nil, service.WithEnumerator(e.disc))
	require.NoError(e.t, err)
	e.svc = svc
}

func (e *env) publish(name, version string, labels map[string]string) *artifact.PublishResult {
	e.t.Helper()
	return testkit.Publish(e.t, e.writer, testkit.Spec{Name: name, Version: version, Content: name + version, Labels: labels})
}

func (e *env) list(request artifact.ListRequest) (*artifact.ListResult, error) {
	e.t.Helper()
	return e.svc.List(context.Background(), request)
}

func (e *env) snapshots() []string {
	files, err := filepath.Glob(filepath.Join(e.dir, "*.json"))
	require.NoError(e.t, err)
	return files
}

func refs(result *artifact.ListResult) []string {
	var out []string
	for index := range result.Artifacts {
		out = append(out, result.Artifacts[index].Repository+"|"+result.Artifacts[index].Name+":"+result.Artifacts[index].Version)
	}
	return out
}

func TestFreshCacheSearchPerformsZeroRegistryRequests(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("components/vpc", "1.0.0", nil)
	e.publish("components/vpc", "1.1.0", nil)
	e.publish("tools/helper", "2.0.0", nil)

	first, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|components/vpc:1.0.0", "shared|components/vpc:1.1.0", "shared|tools/helper:2.0.0"}, refs(first))
	assert.Equal(t, "live", first.Repositories[0].Source)
	assert.NotZero(t, e.registry.Total())
	opened := e.repos.opens.Load()

	e.registry.Reset()
	e.clock.advance(14 * time.Minute)
	found, err := e.svc.Search(context.Background(), artifact.SearchRequest{Query: "vpc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|components/vpc:1.0.0", "shared|components/vpc:1.1.0"}, refs(found))
	assert.Zero(t, e.registry.Total(), "a fresh snapshot answers a search with zero registry requests: %v", e.registry.Requests())
	assert.Equal(t, opened, e.repos.opens.Load(), "a cache hit never opens the repository or resolves its credentials")
	assert.Equal(t, "cache", found.Repositories[0].Source)
	assert.Equal(t, epoch.Format(time.RFC3339), found.Repositories[0].FetchedAt)
	assert.False(t, found.Incomplete)

	// The snapshot keeps repository alias and pinned digests so results stay unambiguous.
	assert.Equal(t, first.Artifacts[0].Pinned, found.Artifacts[0].Pinned)
	assert.NotEmpty(t, found.Artifacts[0].Pinned.ResolvedDigest)
}

func TestStaleSnapshotIsRefreshedAfterTheTTL(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	e.publish("app", "1.1.0", nil)

	e.clock.advance(14 * time.Minute)
	cached, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0"}, refs(cached), "the snapshot is served for fifteen minutes; publishing never edits it")

	e.clock.advance(time.Minute)
	refreshed, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0", "shared|app:1.1.0"}, refs(refreshed))
}

func TestRefreshReusesMetadataByDigestAndReadsNoPayload(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", map[string]string{"team": "platform"})
	e.publish("app", "1.1.0", nil)
	first, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)

	e.rebuild(envOptions{}) // A new process starts with cold in-memory caches.
	e.registry.Reset()
	again, err := e.list(artifact.ListRequest{Refresh: true})
	require.NoError(t, err)
	assert.Equal(t, first.Artifacts, again.Artifacts)
	assert.Equal(t, "live", again.Repositories[0].Source)
	assert.Zero(t, e.registry.Count("GET", "/manifests/"), "unchanged digests reuse metadata: no manifest is read: %v", e.registry.Requests())
	assert.Zero(t, e.registry.Count("GET", "/blobs/"), "no Record config or payload blob is read: %v", e.registry.Requests())
	assert.Equal(t, 2, e.registry.Count("HEAD", "/manifests/"), "each reference is still resolved, so a moved tag is noticed")

	// A changed digest is read again.
	e.publish("app", "1.2.0", nil)
	e.rebuild(envOptions{})
	e.registry.Reset()
	changed, err := e.list(artifact.ListRequest{Refresh: true})
	require.NoError(t, err)
	assert.Len(t, changed.Artifacts, 3)
	assert.Positive(t, e.registry.Count("GET", "/blobs/"), "a new reference's Record is read")
}

func TestDisappearedRefsAreRemovedOnlyAfterACompleteRefresh(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	removed := e.publish("app", "1.1.0", nil)
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)

	pinned := removed.Pinned
	_, err = e.writer.Delete(context.Background(), pinned, artifact.DeleteOptions{Scope: artifact.DeleteManifest})
	require.NoError(t, err)

	// An interrupted refresh reports its error and leaves the snapshot, with the old ref, intact.
	failure := errors.New("connection reset")
	e.registry.FailAfter(2, failure)
	_, err = e.list(artifact.ListRequest{Refresh: true})
	require.ErrorIs(t, err, failure)
	e.registry.FailAll(nil)

	e.registry.Reset()
	stale, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0", "shared|app:1.1.0"}, refs(stale), "a failed refresh never drops refs")
	assert.Zero(t, e.registry.Total())

	complete, err := e.list(artifact.ListRequest{Refresh: true})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0"}, refs(complete), "a complete refresh removes the disappeared ref")
	cached, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, refs(complete), refs(cached))
}

func TestFailedFirstListingIsNeverCachedAsEmpty(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	failure := errors.New("registry unavailable")
	e.registry.FailAll(failure)
	result, err := e.list(artifact.ListRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), failure.Error())
	assert.Empty(t, result.Artifacts)
	assert.True(t, result.Incomplete)
	assert.Empty(t, e.snapshots(), "a failed listing leaves no snapshot")
	e.registry.FailAll(nil)
	ok, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Len(t, ok.Artifacts, 1)
}

func TestOfflineServesSnapshotsWithoutAnyRegistryTraffic(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	_, err = e.list(artifact.ListRequest{Name: "app"})
	require.NoError(t, err)

	e.offline.Store(true)
	e.clock.advance(1000 * time.Hour)
	e.registry.Reset()
	opened := e.repos.opens.Load()
	all, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0"}, refs(all))
	named, err := e.list(artifact.ListRequest{Name: "app"})
	require.NoError(t, err)
	assert.Equal(t, refs(all), refs(named))
	assert.Zero(t, e.registry.Total(), "offline discovery makes no registry request and downloads no payload")
	assert.Equal(t, opened, e.repos.opens.Load())

	_, err = e.list(artifact.ListRequest{Refresh: true})
	require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline, "an offline refresh is rejected")
	assert.Zero(t, e.registry.Total())
}

func TestOfflineRejectsMissingAndCorruptSnapshots(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	e.registry.Reset()
	e.offline.Store(true)
	_, err := e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline, "no snapshot")
	assert.Zero(t, e.registry.Total())

	e.offline.Store(false)
	_, err = e.list(artifact.ListRequest{})
	require.NoError(t, err)
	files := e.snapshots()
	require.Len(t, files, 1)
	original, err := os.ReadFile(files[0])
	require.NoError(t, err)

	e.offline.Store(true)
	for name, corrupt := range map[string]string{
		"truncated":  string(original[:len(original)/2]),
		"empty":      "",
		"garbage":    "not json at all",
		"other kind": strings.Replace(string(original), `"kind":"collection"`, `"kind":"named"`, 1),
		"bad entry":  strings.Replace(string(original), `"name":"app"`, `"name":"UPPER"`, 2),
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, os.WriteFile(files[0], []byte(corrupt), 0o600))
			e.registry.Reset()
			_, err := e.list(artifact.ListRequest{})
			require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline)
			assert.Zero(t, e.registry.Total())
		})
	}
}

func TestOfflineDoesNotCrossAuthRealmsHostBindingsOrConfiguration(t *testing.T) {
	e := newEnv(t, envOptions{scope: "realm-a", identity: "reader-a"})
	e.publish("app", "1.0.0", nil)
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	e.offline.Store(true)
	_, err = e.list(artifact.ListRequest{})
	require.NoError(t, err, "the same scope reuses its snapshot")

	t.Run("another auth realm", func(t *testing.T) {
		other := e.scope
		e.scope = "realm-b"
		e.rebuild(envOptions{})
		_, err := e.list(artifact.ListRequest{})
		require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline)
		e.scope = other
	})
	t.Run("another identity bound to the host", func(t *testing.T) {
		e.info.Discovery.Identity = "reader-b"
		e.rebuild(envOptions{})
		_, err := e.list(artifact.ListRequest{})
		require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline)
		e.info.Discovery.Identity = "reader-a"
	})
	t.Run("another registry location", func(t *testing.T) {
		e.info.Discovery.Location = "other.example.com/acme/atmos"
		e.info.Discovery.Options = map[string]any{"registry": "other.example.com", "prefix": "acme/atmos"}
		e.rebuild(envOptions{})
		_, err := e.list(artifact.ListRequest{})
		require.ErrorIs(t, err, errUtils.ErrArtifactDiscoveryOffline)
	})
}

func TestScopeDoesNotContainCredentialValues(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Artifacts.Repositories = map[string]schema.ArtifactRepositoryConfig{"shared": {Kind: "oci/registry", Identity: "reader"}}
	first, err := discovery.ScopeOf(config)
	require.NoError(t, err)
	again, err := discovery.ScopeOf(config)
	require.NoError(t, err)
	assert.Equal(t, first, again)
	assert.Len(t, first, 64)

	config.Auth.Realm = "other"
	other, err := discovery.ScopeOf(config)
	require.NoError(t, err)
	assert.NotEqual(t, first, other, "the auth realm separates snapshots")

	config.Auth.Realm = ""
	config.Artifacts.Repositories = map[string]schema.ArtifactRepositoryConfig{"shared": {Kind: "oci/registry", Identity: "writer"}}
	rebound, err := discovery.ScopeOf(config)
	require.NoError(t, err)
	assert.NotEqual(t, first, rebound, "the host binding separates snapshots")
}

// rejectCached is a policy that refuses every cached read.
type rejectCached struct{ discovery.LocationPolicy }

var errPolicy = errors.New("source is no longer permitted")

func (rejectCached) AuthorizeSource(_ context.Context, check *discovery.SourceCheck) error {
	if check.Cached {
		return errPolicy
	}
	return nil
}

func TestPolicyIsReevaluatedOnCachedReads(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", nil)
	_, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)

	e.rebuild(envOptions{policy: rejectCached{}})
	e.registry.Reset()
	_, err = e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errPolicy, "a fresh snapshot is not an authorization")
	e.offline.Store(true)
	_, err = e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errPolicy, "offline reads re-evaluate policy too")
	assert.Zero(t, e.registry.Total())
}

func TestNamedListingWorksWithoutACatalogAndCollectionScanFailsLoudly(t *testing.T) {
	e := newEnv(t, envOptions{noCatalog: true})
	e.publish("components/vpc", "1.0.0", nil)
	e.publish("components/vpc", "1.1.0", nil)
	e.publish("tools/helper", "2.0.0", nil)

	named, err := e.list(artifact.ListRequest{Name: "components/vpc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|components/vpc:1.0.0", "shared|components/vpc:1.1.0"}, refs(named))

	result, err := e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "an unsupported collection scan is an error, never an empty success")
	assert.Empty(t, result.Artifacts)
	assert.Contains(t, err.Error(), "catalog")
	assert.Contains(t, err.Error(), "direct named operations")
	assert.True(t, result.Incomplete)
	require.Len(t, result.Repositories, 1)
	assert.NotEmpty(t, result.Repositories[0].Diagnostics)
}

func TestNamedListingIsCachedPerName(t *testing.T) {
	e := newEnv(t, envOptions{noCatalog: true})
	e.publish("app", "1.0.0", nil)
	e.publish("other", "1.0.0", nil)
	_, err := e.list(artifact.ListRequest{Name: "app"})
	require.NoError(t, err)
	e.registry.Reset()
	_, err = e.list(artifact.ListRequest{Name: "app"})
	require.NoError(t, err)
	assert.Zero(t, e.registry.Total())
	_, err = e.list(artifact.ListRequest{Name: "other"})
	require.NoError(t, err)
	assert.Positive(t, e.registry.Total(), "another name has its own snapshot")
}

func TestLabelsUseANDSemanticsOnPublishedVersionMetadata(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("app", "1.0.0", map[string]string{"team": "platform", "env": "dev"})
	e.publish("app", "1.1.0", map[string]string{"team": "platform", "env": "prod"})
	e.publish("other", "1.0.0", map[string]string{"team": "data", "env": "prod"})

	both, err := e.list(artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "platform"}, {Key: "env", Value: "prod"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.1.0"}, refs(both), "every selector must match exactly")
	one, err := e.list(artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "platform"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|app:1.0.0", "shared|app:1.1.0"}, refs(one))
	none, err := e.list(artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "plat"}}})
	require.NoError(t, err)
	assert.Empty(t, none.Artifacts, "label values match exactly, not by substring")

	// Repository defaults apply to later publications only; they never relabel older versions.
	e.info.Defaults = artifact.Metadata{Labels: map[string]string{"retention": "long"}}
	e.rebuild(envOptions{})
	retro, err := e.list(artifact.ListRequest{Labels: []artifact.Label{{Key: "retention", Value: "long"}}})
	require.NoError(t, err)
	assert.Empty(t, retro.Artifacts, "defaults are not retroactively applied to older artifacts")
}

func TestSubstringSearchMatchesNameAndDescriptionCaseInsensitively(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.publish("components/networking", "1.0.0", nil)
	e.publish("tools/helper", "1.0.0", nil)
	byName, err := e.svc.Search(context.Background(), artifact.SearchRequest{Query: "NETWORK"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|components/networking:1.0.0"}, refs(byName))
	byDescription, err := e.svc.Search(context.Background(), artifact.SearchRequest{Query: "package tools"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shared|tools/helper:1.0.0"}, refs(byDescription))
	none, err := e.svc.Search(context.Background(), artifact.SearchRequest{Query: "missing"})
	require.NoError(t, err)
	assert.Empty(t, none.Artifacts)
}

func newLocalRepo(t *testing.T) *local.Repository {
	t.Helper()
	repo, err := local.NewDir("backups", &schema.ArtifactRepositoryConfig{Kind: artifact.RepositoryKindLocalDir, Options: map[string]any{"path": testkit.LocalDir(t)}})
	require.NoError(t, err)
	return repo
}

func TestMultipleRepositoriesSearchTogetherDeterministically(t *testing.T) {
	backups := newLocalRepo(t)
	e := newEnv(t, envOptions{
		extraRepos: map[string]artifact.Repository{"backups": backups},
		extraInfo:  map[string]service.RepositoryInfo{"backups": {Capabilities: backups.Capabilities(), Discovery: service.DiscoveryInfo{Kind: artifact.RepositoryKindLocalDir, Location: "x"}}},
	})
	e.publish("app", "1.0.0", map[string]string{"team": "platform"})
	e.publish("zeta", "1.0.0", nil)
	testkit.Publish(t, backups, testkit.Spec{Name: "app", Version: "1.0.0", Content: "local", Packaging: artifact.PackagingNone, Labels: map[string]string{"team": "platform"}})
	testkit.Publish(t, backups, testkit.Spec{Name: "app", Version: "0.9.0", Content: "older", Packaging: artifact.PackagingNone})

	result, err := e.list(artifact.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"backups|app:0.9.0", "backups|app:1.0.0", "shared|app:1.0.0", "shared|zeta:1.0.0"}, refs(result),
		"results sort by name, then version, then repository alias")
	sources := map[string]string{}
	for _, listing := range result.Repositories {
		sources[listing.Repository] = listing.Source
	}
	assert.Equal(t, map[string]string{"backups": "live", "shared": "live"}, sources, "local repositories are always listed live")

	selected, err := e.list(artifact.ListRequest{Labels: []artifact.Label{{Key: "team", Value: "platform"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"backups|app:1.0.0", "shared|app:1.0.0"}, refs(selected))
	for index := range selected.Artifacts {
		assert.NotEmpty(t, selected.Artifacts[index].Pinned.ResolvedDigest, "each result keeps the repository alias and its pinned digest")
	}

	// A local repository is never served stale after a publish; only remote ones are cached.
	testkit.Publish(t, backups, testkit.Spec{Name: "fresh", Version: "1.0.0", Content: "new", Packaging: artifact.PackagingNone})
	again, err := e.list(artifact.ListRequest{Repositories: []string{"backups"}})
	require.NoError(t, err)
	assert.Contains(t, refs(again), "backups|fresh:1.0.0")
}

func TestUnauthorizedOrFailingRepositoryIsNeverPresentedAsComplete(t *testing.T) {
	backups := newLocalRepo(t)
	e := newEnv(t, envOptions{
		extraRepos: map[string]artifact.Repository{"backups": backups},
		extraInfo:  map[string]service.RepositoryInfo{"backups": {Capabilities: backups.Capabilities(), Discovery: service.DiscoveryInfo{Kind: artifact.RepositoryKindLocalDir, Location: "x"}}},
	})
	testkit.Publish(t, backups, testkit.Spec{Name: "local-app", Version: "1.0.0", Content: "x", Packaging: artifact.PackagingNone})
	e.publish("app", "1.0.0", nil)
	e.registry.FailAll(errors.New("401 unauthorized"))

	result, err := e.list(artifact.ListRequest{})
	require.NoError(t, err, "the readable repository is still listed")
	assert.Equal(t, []string{"backups|local-app:1.0.0"}, refs(result))
	assert.True(t, result.Incomplete, "the result is flagged incomplete")
	var failing artifact.RepositoryListing
	for _, listing := range result.Repositories {
		if listing.Repository == "shared" {
			failing = listing
		}
	}
	require.NotEmpty(t, failing.Diagnostics)
	assert.Equal(t, artifact.SeverityError, failing.Diagnostics[0].Severity)
	assert.Contains(t, failing.Diagnostics[0].Message, "401")

	_, err = e.list(artifact.ListRequest{Repositories: []string{"shared"}})
	require.Error(t, err, "an explicitly requested repository that fails fails the command")
}

func TestPolicyDefaultRejectsUnreadableRepositories(t *testing.T) {
	e := newEnv(t, envOptions{})
	e.info.Capabilities.Read = false
	e.rebuild(envOptions{})
	_, err := e.list(artifact.ListRequest{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
}
