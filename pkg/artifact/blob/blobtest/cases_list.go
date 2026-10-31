package blobtest

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/blob"
)

func (c *conformance) listPrefix(t *testing.T) {
	store := c.newStore(t)
	for _, key := range []string{"a/2", "a/1", "b/1", "a/sub/3", "ab"} {
		put(t, store, key, []byte(key))
	}
	objects, prefixes, err := blob.ListAll(context.Background(), store, blob.ListOptions{Prefix: "a/"})
	require.NoError(t, err)
	assert.Empty(t, prefixes)
	assert.Equal(t, []string{"a/1", "a/2", "a/sub/3"}, keysOf(objects), "lexicographic order, prefix-scoped")
	for _, object := range objects {
		assert.Equal(t, int64(len(object.Key)), object.Size)
	}

	objects, _, err = blob.ListAll(context.Background(), store, blob.ListOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"a/1", "a/2", "a/sub/3", "ab", "b/1"}, keysOf(objects))

	objects, _, err = blob.ListAll(context.Background(), store, blob.ListOptions{Prefix: "zzz/"})
	require.NoError(t, err)
	assert.Empty(t, objects)
}

func (c *conformance) listPagination(t *testing.T) {
	store := c.newStore(t)
	const total = listKeyCount
	want := make([]string, 0, total)
	for i := range total {
		key := fmt.Sprintf("page/%04d", i)
		want = append(want, key)
		put(t, store, key, nil)
	}
	slices.Sort(want)

	var (
		got    []string
		cursor string
		pages  int
	)
	for {
		page, err := store.List(context.Background(), blob.ListOptions{Prefix: "page/", Cursor: cursor, Limit: listPageSize})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(page.Objects), listPageSize, "a page never exceeds the limit")
		got = append(got, keysOf(page.Objects)...)
		pages++
		require.Less(t, pages, listPageGuard, "pagination must terminate")
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	assert.Equal(t, want, got, "every key exactly once, in order")
	assert.GreaterOrEqual(t, pages, listMinPages, "53 keys at 10 per page need at least 6 pages")
}

func (c *conformance) listDelimiter(t *testing.T) {
	store := c.newStore(t)
	if !store.Capabilities().ListDelimiter {
		t.Skipf("store does not advertise delimiter listing")
	}
	for _, key := range []string{"root.txt", "x/1", "x/2", "x/deep/3", "y/1", "z"} {
		put(t, store, key, nil)
	}
	objects, prefixes, err := blob.ListAll(context.Background(), store, blob.ListOptions{Delimiter: "/"})
	require.NoError(t, err)
	assert.Equal(t, []string{"root.txt", "z"}, keysOf(objects))
	assert.Equal(t, []string{"x/", "y/"}, prefixes)

	// Paging must not split or repeat a rolled-up prefix.
	var (
		gotObjects  []string
		gotPrefixes []string
		cursor      string
	)
	for range delimiterPages {
		page, err := store.List(context.Background(), blob.ListOptions{Delimiter: "/", Cursor: cursor, Limit: 1})
		require.NoError(t, err)
		gotObjects = append(gotObjects, keysOf(page.Objects)...)
		gotPrefixes = append(gotPrefixes, page.CommonPrefixes...)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	assert.Equal(t, []string{"root.txt", "z"}, gotObjects)
	assert.Equal(t, []string{"x/", "y/"}, gotPrefixes)

	objects, prefixes, err = blob.ListAll(context.Background(), store, blob.ListOptions{Prefix: "x/", Delimiter: "/"})
	require.NoError(t, err)
	assert.Equal(t, []string{"x/1", "x/2"}, keysOf(objects))
	assert.Equal(t, []string{"x/deep/"}, prefixes)
}
