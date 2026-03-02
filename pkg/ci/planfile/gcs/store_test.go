package gcs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
)

const testBucket = "test-planfiles"

// newTestStore creates a Store backed by fake-gcs-server for testing.
func newTestStore(t *testing.T, prefix string) (*Store, *fakestorage.Server) {
	t.Helper()

	server := fakestorage.NewServer(nil)
	server.CreateBucket(testBucket)
	t.Cleanup(server.Stop)

	return &Store{
		client: server.Client(),
		bucket: testBucket,
		prefix: prefix,
	}, server
}

func TestStore_Name(t *testing.T) {
	store := &Store{bucket: "test-bucket"}
	assert.Equal(t, "gcs", store.Name())
}

func TestStore_fullKey(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		key      string
		expected string
	}{
		{
			name:     "no prefix",
			prefix:   "",
			key:      "stack/component/sha.tfplan",
			expected: "stack/component/sha.tfplan",
		},
		{
			name:     "with prefix",
			prefix:   "planfiles",
			key:      "stack/component/sha.tfplan",
			expected: "planfiles/stack/component/sha.tfplan",
		},
		{
			name:     "nested prefix",
			prefix:   "atmos/ci/plans",
			key:      "dev/vpc/abc.tfplan",
			expected: "atmos/ci/plans/dev/vpc/abc.tfplan",
		},
		{
			name:     "simple key no prefix",
			prefix:   "",
			key:      "test.tfplan",
			expected: "test.tfplan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &Store{prefix: tt.prefix}
			result := store.fullKey(tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewStore_MissingBucket(t *testing.T) {
	tests := []struct {
		name    string
		options map[string]any
	}{
		{
			name:    "nil options",
			options: nil,
		},
		{
			name:    "empty options",
			options: map[string]any{},
		},
		{
			name: "empty bucket",
			options: map[string]any{
				"bucket": "",
			},
		},
		{
			name: "wrong type bucket",
			options: map[string]any{
				"bucket": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStore(planfile.StoreOptions{
				Options: tt.options,
			})
			assert.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileStoreInvalidArgs)
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "other error",
			err:      errors.New("some error"),
			expected: false,
		},
		{
			name:     "ErrObjectNotExist",
			err:      storage.ErrObjectNotExist,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStore_MetadataSuffix(t *testing.T) {
	assert.Equal(t, ".metadata.json", metadataSuffix)
}

func TestStore_StoreName(t *testing.T) {
	assert.Equal(t, "gcs", storeName)
}

func TestStore_Upload(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	t.Run("upload without metadata", func(t *testing.T) {
		err := store.Upload(ctx, "test/plan.tfplan", strings.NewReader("plan data"), nil)
		require.NoError(t, err)

		// Verify the object was created.
		exists, err := store.Exists(ctx, "test/plan.tfplan")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("upload with metadata", func(t *testing.T) {
		metadata := &planfile.Metadata{
			Stack:      "plat-ue2-dev",
			Component:  "vpc",
			SHA:        "abc123",
			HasChanges: true,
			Additions:  3,
			Changes:    1,
		}
		err := store.Upload(ctx, "test/plan-with-meta.tfplan", strings.NewReader("plan data"), metadata)
		require.NoError(t, err)

		// Verify metadata sidecar was created.
		exists, err := store.Exists(ctx, "test/plan-with-meta.tfplan")
		require.NoError(t, err)
		assert.True(t, exists)

		// Verify metadata can be loaded.
		meta, err := store.loadMetadata(ctx, "test/plan-with-meta.tfplan")
		require.NoError(t, err)
		assert.Equal(t, "plat-ue2-dev", meta.Stack)
		assert.Equal(t, "vpc", meta.Component)
		assert.Equal(t, "abc123", meta.SHA)
		assert.True(t, meta.HasChanges)
		assert.Equal(t, 3, meta.Additions)
		assert.Equal(t, 1, meta.Changes)
	})
}

func TestStore_Upload_WithPrefix(t *testing.T) {
	store, _ := newTestStore(t, "ci/plans")
	ctx := context.Background()

	err := store.Upload(ctx, "stack/vpc/abc.tfplan", strings.NewReader("prefixed plan"), nil)
	require.NoError(t, err)

	// Verify the object exists under the prefix.
	exists, err := store.Exists(ctx, "stack/vpc/abc.tfplan")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestStore_Download(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	t.Run("download existing file", func(t *testing.T) {
		// Upload first.
		metadata := &planfile.Metadata{
			Stack:     "plat-ue2-dev",
			Component: "vpc",
			SHA:       "abc123",
		}
		err := store.Upload(ctx, "download/test.tfplan", strings.NewReader("download content"), metadata)
		require.NoError(t, err)

		// Download.
		reader, meta, err := store.Download(ctx, "download/test.tfplan")
		require.NoError(t, err)
		defer reader.Close()

		content, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, "download content", string(content))
		assert.Equal(t, "plat-ue2-dev", meta.Stack)
		assert.Equal(t, "vpc", meta.Component)
	})

	t.Run("download nonexistent file", func(t *testing.T) {
		_, _, err := store.Download(ctx, "does/not/exist.tfplan")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileNotFound)
	})
}

func TestStore_Delete(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	t.Run("delete existing file", func(t *testing.T) {
		// Upload first.
		err := store.Upload(ctx, "delete/test.tfplan", strings.NewReader("to be deleted"), &planfile.Metadata{
			Stack: "test",
		})
		require.NoError(t, err)

		// Verify it exists.
		exists, err := store.Exists(ctx, "delete/test.tfplan")
		require.NoError(t, err)
		assert.True(t, exists)

		// Delete.
		err = store.Delete(ctx, "delete/test.tfplan")
		require.NoError(t, err)

		// Verify deleted.
		exists, err = store.Exists(ctx, "delete/test.tfplan")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("delete nonexistent file is idempotent", func(t *testing.T) {
		err := store.Delete(ctx, "does/not/exist.tfplan")
		assert.NoError(t, err)
	})
}

func TestStore_List(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	// Upload several files.
	files := []struct {
		key     string
		content string
	}{
		{"list/stack1/vpc/abc.tfplan", "plan1"},
		{"list/stack1/rds/abc.tfplan", "plan2"},
		{"list/stack2/vpc/def.tfplan", "plan3"},
		{"other/stack3/vpc/ghi.tfplan", "plan4"},
	}

	for _, f := range files {
		err := store.Upload(ctx, f.key, strings.NewReader(f.content), nil)
		require.NoError(t, err)
	}

	t.Run("list with prefix", func(t *testing.T) {
		results, err := store.List(ctx, "list/stack1/")
		require.NoError(t, err)
		assert.Len(t, results, 2)

		// Verify keys are returned.
		keys := make([]string, len(results))
		for i, r := range results {
			keys[i] = r.Key
		}
		assert.Contains(t, keys, "list/stack1/vpc/abc.tfplan")
		assert.Contains(t, keys, "list/stack1/rds/abc.tfplan")
	})

	t.Run("list all", func(t *testing.T) {
		results, err := store.List(ctx, "list/")
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("list empty prefix returns no results", func(t *testing.T) {
		results, err := store.List(ctx, "nonexistent/")
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

func TestStore_List_WithPrefix(t *testing.T) {
	store, _ := newTestStore(t, "ci/plans")
	ctx := context.Background()

	// Upload files with store prefix.
	err := store.Upload(ctx, "stack/vpc/abc.tfplan", strings.NewReader("plan1"), nil)
	require.NoError(t, err)
	err = store.Upload(ctx, "stack/rds/abc.tfplan", strings.NewReader("plan2"), nil)
	require.NoError(t, err)

	results, err := store.List(ctx, "stack/")
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify relative keys (prefix stripped).
	for _, r := range results {
		assert.False(t, strings.HasPrefix(r.Key, "ci/plans/"), "key should not include store prefix: %s", r.Key)
	}
}

func TestStore_List_SkipsMetadataFiles(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	// Upload a planfile with metadata.
	err := store.Upload(ctx, "meta/test.tfplan", strings.NewReader("plan"), &planfile.Metadata{
		Stack:     "test",
		Component: "vpc",
	})
	require.NoError(t, err)

	// List should only return the planfile, not the .metadata.json sidecar.
	results, err := store.List(ctx, "meta/")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "meta/test.tfplan", results[0].Key)
}

func TestStore_Exists(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	t.Run("exists returns true for existing object", func(t *testing.T) {
		err := store.Upload(ctx, "exists/test.tfplan", strings.NewReader("data"), nil)
		require.NoError(t, err)

		exists, err := store.Exists(ctx, "exists/test.tfplan")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("exists returns false for nonexistent object", func(t *testing.T) {
		exists, err := store.Exists(ctx, "does/not/exist.tfplan")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestStore_GetMetadata(t *testing.T) {
	store, _ := newTestStore(t, "")
	ctx := context.Background()

	t.Run("returns full metadata when sidecar exists", func(t *testing.T) {
		metadata := &planfile.Metadata{
			Stack:        "plat-ue2-dev",
			Component:    "vpc",
			SHA:          "abc123",
			HasChanges:   true,
			Additions:    5,
			Changes:      2,
			Destructions: 1,
			CreatedAt:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		}
		err := store.Upload(ctx, "getmeta/test.tfplan", strings.NewReader("plan"), metadata)
		require.NoError(t, err)

		meta, err := store.GetMetadata(ctx, "getmeta/test.tfplan")
		require.NoError(t, err)
		assert.Equal(t, "plat-ue2-dev", meta.Stack)
		assert.Equal(t, "vpc", meta.Component)
		assert.Equal(t, "abc123", meta.SHA)
		assert.True(t, meta.HasChanges)
		assert.Equal(t, 5, meta.Additions)
		assert.Equal(t, 2, meta.Changes)
		assert.Equal(t, 1, meta.Destructions)
	})

	t.Run("returns minimal metadata when no sidecar", func(t *testing.T) {
		// Upload without metadata.
		err := store.Upload(ctx, "getmeta/no-sidecar.tfplan", strings.NewReader("plan"), nil)
		require.NoError(t, err)

		meta, err := store.GetMetadata(ctx, "getmeta/no-sidecar.tfplan")
		require.NoError(t, err)
		assert.NotNil(t, meta)
		// CreatedAt should be set from GCS object attributes.
		assert.False(t, meta.CreatedAt.IsZero())
	})

	t.Run("returns error for nonexistent object", func(t *testing.T) {
		_, err := store.GetMetadata(ctx, "does/not/exist.tfplan")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanfileNotFound)
	})
}

func TestStore_FullLifecycle(t *testing.T) {
	store, _ := newTestStore(t, "lifecycle")
	ctx := context.Background()
	key := "plat-ue2-dev/vpc/abc123.tfplan"

	// 1. Upload.
	metadata := &planfile.Metadata{
		Stack:      "plat-ue2-dev",
		Component:  "vpc",
		SHA:        "abc123",
		HasChanges: true,
		Additions:  3,
	}
	err := store.Upload(ctx, key, strings.NewReader("terraform plan output"), metadata)
	require.NoError(t, err)

	// 2. Exists.
	exists, err := store.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// 3. GetMetadata.
	meta, err := store.GetMetadata(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "plat-ue2-dev", meta.Stack)
	assert.Equal(t, "vpc", meta.Component)

	// 4. Download.
	reader, dlMeta, err := store.Download(ctx, key)
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	reader.Close()
	assert.Equal(t, "terraform plan output", string(content))
	assert.Equal(t, "plat-ue2-dev", dlMeta.Stack)

	// 5. List.
	files, err := store.List(ctx, "plat-ue2-dev/")
	require.NoError(t, err)
	assert.Len(t, files, 1)

	// 6. Delete.
	err = store.Delete(ctx, key)
	require.NoError(t, err)

	// 7. Verify deleted.
	exists, err = store.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}
