package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/planfile"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name        string
		opts        planfile.StoreOptions
		expectError bool
	}{
		{
			name: "valid options",
			opts: planfile.StoreOptions{
				Options: map[string]any{
					"path": "/tmp/planfiles",
				},
			},
			expectError: false,
		},
		{
			name: "default path",
			opts: planfile.StoreOptions{
				Options: map[string]any{},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewStore(tt.opts)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, store)
			}
		})
	}
}

func TestStore_Name(t *testing.T) {
	store := &Store{basePath: "/tmp"}
	assert.Equal(t, "local", store.Name())
}

func TestStore_UploadDownload(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Test data.
	key := "test-stack/test-component/abc123.tfplan"
	content := "plan file content"
	metadata := &planfile.Metadata{
		Stack:      "test-stack",
		Component:  "test-component",
		SHA:        "abc123",
		CreatedAt:  time.Now(),
		HasChanges: true,
		Additions:  5,
	}

	// Upload.
	err := store.Upload(ctx, key, strings.NewReader(content), metadata)
	assert.NoError(t, err)

	// Verify file exists.
	exists, err := store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Download.
	reader, downloadedMeta, err := store.Download(ctx, key)
	assert.NoError(t, err)
	defer reader.Close()

	// Verify content.
	downloadedContent := make([]byte, len(content))
	_, err = reader.Read(downloadedContent)
	assert.NoError(t, err)
	assert.Equal(t, content, string(downloadedContent))

	// Verify metadata.
	assert.Equal(t, metadata.Stack, downloadedMeta.Stack)
	assert.Equal(t, metadata.Component, downloadedMeta.Component)
	assert.Equal(t, metadata.SHA, downloadedMeta.SHA)
	assert.Equal(t, metadata.HasChanges, downloadedMeta.HasChanges)
	assert.Equal(t, metadata.Additions, downloadedMeta.Additions)
}

func TestStore_Delete(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Create a file.
	key := "to-delete.tfplan"
	err := store.Upload(ctx, key, strings.NewReader("content"), nil)
	require.NoError(t, err)

	// Verify it exists.
	exists, err := store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Delete.
	err = store.Delete(ctx, key)
	assert.NoError(t, err)

	// Verify it's gone.
	exists, err = store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_List(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload several files.
	files := []string{
		"stack1/component1/abc.tfplan",
		"stack1/component2/def.tfplan",
		"stack2/component1/ghi.tfplan",
	}

	for _, key := range files {
		err := store.Upload(ctx, key, strings.NewReader("content"), nil)
		require.NoError(t, err)
	}

	// List all.
	list, err := store.List(ctx, "")
	assert.NoError(t, err)
	assert.Len(t, list, 3)

	// List with prefix.
	list, err = store.List(ctx, "stack1")
	assert.NoError(t, err)
	assert.Len(t, list, 2)

	// List specific component.
	list, err = store.List(ctx, "stack1/component1")
	assert.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestStore_Exists(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Create a file.
	key := "exists.tfplan"
	err := store.Upload(ctx, key, strings.NewReader("content"), nil)
	require.NoError(t, err)

	// Test exists.
	exists, err := store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test not exists.
	exists, err = store.Exists(ctx, "nonexistent.tfplan")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_GetMetadata(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload with metadata.
	key := "meta.tfplan"
	metadata := &planfile.Metadata{
		Stack:        "test-stack",
		Component:    "test-component",
		SHA:          "sha123",
		HasChanges:   true,
		Additions:    3,
		Changes:      2,
		Destructions: 1,
	}
	err := store.Upload(ctx, key, strings.NewReader("content"), metadata)
	require.NoError(t, err)

	// Get metadata.
	retrieved, err := store.GetMetadata(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, metadata.Stack, retrieved.Stack)
	assert.Equal(t, metadata.Component, retrieved.Component)
	assert.Equal(t, metadata.SHA, retrieved.SHA)
	assert.Equal(t, metadata.HasChanges, retrieved.HasChanges)
	assert.Equal(t, metadata.Additions, retrieved.Additions)
	assert.Equal(t, metadata.Changes, retrieved.Changes)
	assert.Equal(t, metadata.Destructions, retrieved.Destructions)
}

func TestIntegration_FullLifecycle(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()

	// Create store.
	store, err := NewStore(planfile.StoreOptions{
		Options: map[string]any{
			"path": tmpDir,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	key := "integration/test/lifecycle.tfplan"

	// 1. Upload.
	metadata := &planfile.Metadata{
		Stack:      "integration",
		Component:  "test",
		SHA:        "lifecycle",
		HasChanges: true,
	}
	err = store.Upload(ctx, key, strings.NewReader("plan data"), metadata)
	assert.NoError(t, err)

	// 2. Verify exists.
	exists, err := store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.True(t, exists)

	// 3. Get metadata.
	meta, err := store.GetMetadata(ctx, key)
	assert.NoError(t, err)
	assert.Equal(t, "integration", meta.Stack)

	// 4. List.
	list, err := store.List(ctx, "integration")
	assert.NoError(t, err)
	assert.Len(t, list, 1)

	// 5. Download.
	reader, _, err := store.Download(ctx, key)
	assert.NoError(t, err)
	reader.Close()

	// 6. Delete.
	err = store.Delete(ctx, key)
	assert.NoError(t, err)

	// 7. Verify deleted.
	exists, err = store.Exists(ctx, key)
	assert.NoError(t, err)
	assert.False(t, exists)

	// Verify metadata file is also cleaned up.
	metaPath := filepath.Join(tmpDir, ".meta", key+".json")
	_, err = os.Stat(metaPath)
	assert.True(t, os.IsNotExist(err))
}
