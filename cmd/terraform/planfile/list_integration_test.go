package planfile

import (
	"bytes"
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// localStoreConfig builds an AtmosConfiguration whose named "local" planfile store
// is backed by the on-disk filesystem at dir. Driving createStore against this
// exercises the real registry → local backend → adapter path (no mocks), so the
// list assertions below are empirical end-to-end checks of store.List().
func localStoreConfig(dir string) *schema.AtmosConfiguration {
	c := &schema.AtmosConfiguration{}
	c.Components.Terraform.Planfiles.Stores = map[string]schema.PlanfileStoreSpec{
		"local": {Type: "local/dir", Options: map[string]any{"path": dir}},
	}
	return c
}

// uploadFixturePlanfile writes a single planfile (one fake .tfplan file plus its
// metadata) to the store so List can later find and filter it.
func uploadFixturePlanfile(t *testing.T, store planfile.Store, component, stack, sha string) {
	t.Helper()

	key := stack + "/" + component + "/" + sha + ".tfplan.tar"
	files := []planfile.FileEntry{
		{Name: "plan.tfplan", Data: bytes.NewReader([]byte("fake-plan-" + key)), Size: int64(len("fake-plan-" + key))},
	}
	md := &planfile.Metadata{}
	md.Stack = stack
	md.Component = component
	md.SHA = sha
	md.CreatedAt = time.Now()

	require.NoError(t, store.Upload(context.Background(), key, files, md))
}

// coord is a comparable (component, stack, sha) tuple used to assert list contents
// by value rather than by length alone.
type coord struct{ component, stack, sha string }

func coordsOf(t *testing.T, files []planfile.PlanfileInfo) []coord {
	t.Helper()
	out := make([]coord, 0, len(files))
	for _, f := range files {
		require.NotNil(t, f.Metadata)
		out = append(out, coord{f.Metadata.Component, f.Metadata.Stack, f.Metadata.SHA})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].component != out[j].component {
			return out[i].component < out[j].component
		}
		if out[i].stack != out[j].stack {
			return out[i].stack < out[j].stack
		}
		return out[i].sha < out[j].sha
	})
	return out
}

func TestRunList_LocalStore_Empirical(t *testing.T) {
	dir := t.TempDir()
	atmosConfig := localStoreConfig(dir)

	store, err := createStore(atmosConfig, "local")
	require.NoError(t, err)

	// Three planfiles spanning two components, two stacks, and two SHAs.
	uploadFixturePlanfile(t, store, "rds", "prod", "bbb")
	uploadFixturePlanfile(t, store, "vpc", "prod", "aaa")
	uploadFixturePlanfile(t, store, "vpc", "staging", "aaa")

	ctx := context.Background()

	t.Run("all returns every planfile, contents asserted by value", func(t *testing.T) {
		files, err := store.List(ctx, planfile.Query{All: true})
		require.NoError(t, err)
		got := coordsOf(t, files)
		require.Len(t, got, 3)
		// Assert first and last element by value (not just length).
		assert.Equal(t, coord{"rds", "prod", "bbb"}, got[0])
		assert.Equal(t, coord{"vpc", "staging", "aaa"}, got[2])
		assert.Equal(t, coord{"vpc", "prod", "aaa"}, got[1])
	})

	t.Run("component filter", func(t *testing.T) {
		files, err := store.List(ctx, buildQuery("vpc", "", ""))
		require.NoError(t, err)
		assert.Equal(t, []coord{{"vpc", "prod", "aaa"}, {"vpc", "staging", "aaa"}}, coordsOf(t, files))
	})

	t.Run("stack filter", func(t *testing.T) {
		files, err := store.List(ctx, buildQuery("", "prod", ""))
		require.NoError(t, err)
		assert.Equal(t, []coord{{"rds", "prod", "bbb"}, {"vpc", "prod", "aaa"}}, coordsOf(t, files))
	})

	t.Run("sha filter", func(t *testing.T) {
		files, err := store.List(ctx, buildQuery("", "", "bbb"))
		require.NoError(t, err)
		assert.Equal(t, []coord{{"rds", "prod", "bbb"}}, coordsOf(t, files))
	})

	t.Run("component and stack filter", func(t *testing.T) {
		files, err := store.List(ctx, buildQuery("vpc", "prod", ""))
		require.NoError(t, err)
		assert.Equal(t, []coord{{"vpc", "prod", "aaa"}}, coordsOf(t, files))
	})

	t.Run("no match returns empty", func(t *testing.T) {
		files, err := store.List(ctx, buildQuery("nonexistent", "", ""))
		require.NoError(t, err)
		assert.Empty(t, coordsOf(t, files))
	})
}
