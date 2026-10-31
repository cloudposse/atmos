package oci

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
)

// withoutCatalog hides the repository catalog of a transport, like a registry that has none.
type withoutCatalog struct{ atmosoci.Transport }

// failingCatalog implements the catalog but the registry rejects it.
type failingCatalog struct {
	atmosoci.Transport
	err error
}

func (f failingCatalog) Repositories(context.Context, string) ([]string, error) { return nil, f.err }

func listAll(t *testing.T, repo *Repository, query artifact.ListQuery) []string {
	t.Helper()
	var seen []string
	for pages := 0; pages < 50; pages++ {
		page, err := repo.List(context.Background(), query)
		require.NoError(t, err)
		for index := range page.Entries {
			seen = append(seen, page.Entries[index].Name+":"+page.Entries[index].Version)
		}
		if page.NextCursor == "" {
			return seen
		}
		require.NotEqual(t, query.Cursor, page.NextCursor)
		query.Cursor = page.NextCursor
	}
	t.Fatal("pagination did not terminate")
	return nil
}

func TestCollectionListingIsScopedSortedAndPaged(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		for _, item := range []struct{ name, version string }{{"b/app", "1"}, {"a/app", "2"}, {"a/app", "1"}, {"a/other", "3"}} {
			publish(t, repo, publishSpec{name: item.name, version: item.version, content: item.name + item.version})
		}
		want := []string{"a/app:1", "a/app:2", "a/other:3", "b/app:1"}
		assert.Equal(t, want, listAll(t, repo, artifact.ListQuery{PageSize: 100}))
		assert.Equal(t, want, listAll(t, repo, artifact.ListQuery{PageSize: 1}), "every entry exactly once across pages")
		assert.Equal(t, []string{"a/app:1", "a/app:2"}, listAll(t, repo, artifact.ListQuery{Name: "a/app", PageSize: 1}))
	})
}

func TestRegistryListingStaysInsideThePrefix(t *testing.T) {
	reg := newRegistry(t, nil)
	inside := newRegistryRepo(t, reg)
	publish(t, inside, publishSpec{name: "mine/app", content: "inside"})

	otherConfig := registryConfig(reg.server.Host)
	otherConfig.Options["prefix"] = "somebody/else"
	outside, err := NewRepository(context.Background(), "outside", otherConfig, reg.options(WithLockDir(t.TempDir()))...)
	require.NoError(t, err)
	publish(t, outside, publishSpec{name: "theirs/app", content: "outside"})

	assert.Equal(t, []string{"mine/app:1.0.0"}, listAll(t, inside, artifact.ListQuery{}))
	assert.Equal(t, []string{"theirs/app:1.0.0"}, listAll(t, outside, artifact.ListQuery{}))
}

func TestLabelFilterUsesExactPairsWithAndSemantics(t *testing.T) {
	backends(t, func(t *testing.T, repo *Repository) {
		publish(t, repo, publishSpec{name: "a/one", content: "1", labels: map[string]string{"team": "platform", "purpose": "backup"}})
		publish(t, repo, publishSpec{name: "a/two", content: "2", labels: map[string]string{"team": "platform"}})
		publish(t, repo, publishSpec{name: "a/three", content: "3", labels: map[string]string{"team": "data", "purpose": "backup"}})

		assert.Equal(t, []string{"a/one:1.0.0", "a/two:1.0.0"}, listAll(t, repo, artifact.ListQuery{Labels: []artifact.Label{{Key: "team", Value: "platform"}}}))
		assert.Equal(t, []string{"a/one:1.0.0"}, listAll(t, repo, artifact.ListQuery{Labels: []artifact.Label{{Key: "team", Value: "platform"}, {Key: "purpose", Value: "backup"}}}))
		assert.Empty(t, listAll(t, repo, artifact.ListQuery{Labels: []artifact.Label{{Key: "team", Value: "plat"}}}), "values match exactly")
	})
}

// A registry without a catalog still lists a named artifact's tags; the collection scan is a
// capability error that says so, never an empty successful result.
func TestNamedListingWorksWithoutACatalogAndCollectionListingFailsLoudly(t *testing.T) {
	reg := newRegistry(t, nil)
	writer := newRegistryRepo(t, reg)
	publish(t, writer, publishSpec{content: "one"})
	publish(t, writer, publishSpec{version: "2.0.0", content: "two"})

	tests := map[string]atmosoci.Transport{
		"transport without a catalog": withoutCatalog{reg.transport},
		"registry rejects the catalog": failingCatalog{
			Transport: reg.transport, err: errors.New("GET _catalog: response status code 404"),
		},
	}
	for name, transport := range tests {
		t.Run(name, func(t *testing.T) {
			repo, err := NewRepository(context.Background(), "registry", registryConfig(reg.server.Host), WithTransport(transport), WithLockDir(t.TempDir()))
			require.NoError(t, err)
			assert.Equal(t, []string{"team/app:1.0.0", "team/app:2.0.0"}, listAll(t, repo, artifact.ListQuery{Name: appName}))

			_, err = repo.List(context.Background(), artifact.ListQuery{})
			require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
			assert.Contains(t, err.Error(), "direct named operation")
		})
	}
	caps := mustCaps(t, reg, WithoutCollectionEnumeration())
	assert.False(t, caps.EnumerateCollection)
	assert.True(t, caps.EnumerateNamed)
}

func mustCaps(t *testing.T, reg *registry, opts ...Option) artifact.Capabilities {
	t.Helper()
	repo, err := NewRepository(context.Background(), "r", registryConfig(reg.server.Host), append(reg.options(WithLockDir(t.TempDir())), opts...)...)
	require.NoError(t, err)
	return repo.Capabilities()
}

func TestListingNeverPresentsForeignImagesOrPluginsAsArtifacts(t *testing.T) {
	reg := newRegistry(t, nil)
	repo := newRegistryRepo(t, reg)
	publish(t, repo, publishSpec{name: "shared/thing", version: "1.0.0", content: "generic"})
	pushImage(t, reg, testPrefix+"/shared/thing", "image")
	importPlugin(t, repo, artifact.Ref{Name: "shared/thing", Version: "plugin"})

	assert.Equal(t, []string{"shared/thing:1.0.0"}, listAll(t, repo, artifact.ListQuery{Name: "shared/thing"}))
	assert.Equal(t, []string{"shared/thing:1.0.0"}, listAll(t, repo, artifact.ListQuery{}))
}
