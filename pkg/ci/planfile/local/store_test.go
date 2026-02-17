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

	errUtils "github.com/cloudposse/atmos/errors"
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

func TestStore_Download_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Try to download a non-existent file.
	_, _, err := store.Download(ctx, "nonexistent/file.tfplan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestStore_Delete_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Delete should be idempotent - deleting non-existent file is OK.
	err := store.Delete(ctx, "nonexistent/file.tfplan")
	assert.NoError(t, err)
}

func TestStore_GetMetadata_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Try to get metadata for non-existent file.
	_, err := store.GetMetadata(ctx, "nonexistent/file.tfplan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestStore_GetMetadata_NoMetadataFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Create a planfile without metadata.
	key := "no-meta.tfplan"
	err := store.Upload(ctx, key, strings.NewReader("content"), nil)
	require.NoError(t, err)

	// Get metadata should return file info-based metadata.
	meta, err := store.GetMetadata(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, meta)
	// Should have CreatedAt from file modtime.
	assert.False(t, meta.CreatedAt.IsZero())
}

func TestStore_List_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload some files.
	err := store.Upload(ctx, "stack1/test.tfplan", strings.NewReader("content"), nil)
	require.NoError(t, err)

	// List with non-matching prefix.
	list, err := store.List(ctx, "nonexistent-prefix")
	assert.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_List_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// List empty directory.
	list, err := store.List(ctx, "")
	assert.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_Upload_CreatesDirs(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload to a deeply nested path.
	key := "deeply/nested/path/to/file.tfplan"
	err := store.Upload(ctx, key, strings.NewReader("content"), nil)
	assert.NoError(t, err)

	// Verify the directories were created.
	fullPath := filepath.Join(tmpDir, key)
	_, err = os.Stat(fullPath)
	assert.NoError(t, err)
}

func TestStore_Delete_CleansUpEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload to nested directory.
	key := "cleanup/test/nested/file.tfplan"
	err := store.Upload(ctx, key, strings.NewReader("content"), nil)
	require.NoError(t, err)

	// Delete the file.
	err = store.Delete(ctx, key)
	assert.NoError(t, err)

	// Verify empty parent directories were cleaned up.
	// The "cleanup" directory should be removed since it's empty.
	cleanupDir := filepath.Join(tmpDir, "cleanup")
	_, err = os.Stat(cleanupDir)
	assert.True(t, os.IsNotExist(err), "expected cleanup dir to be removed")
}

func TestNewStore_WithTildePath(t *testing.T) {
	// Test that ~ is expanded to home directory.
	opts := planfile.StoreOptions{
		Options: map[string]any{
			"path": "~/.atmos-test-planfiles",
		},
	}

	store, err := NewStore(opts)
	require.NoError(t, err)
	require.NotNil(t, store)

	// Clean up.
	localStore := store.(*Store)
	_ = os.RemoveAll(localStore.basePath)
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s      string
		prefix string
		want   bool
	}{
		{"stack1/component1", "stack1", true},
		{"stack1/component1", "stack2", false},
		{"stack1/component1", "stack1/component1", true},
		{"stack1", "stack1/component1", false},
		{"", "", true},
		{"abc", "", true},
		{"", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.prefix, func(t *testing.T) {
			got := hasPrefix(tt.s, tt.prefix)
			assert.Equal(t, tt.want, got)
		})
	}
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

func TestStore_PathTraversal(t *testing.T) {
	// Create temp directory.
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Test that path traversal attempts are rejected for all operations.
	maliciousKeys := []string{
		"../escape",
		"../../etc/passwd",
		"valid/../../../escape",
		"stack/../../../etc/passwd",
	}

	for _, key := range maliciousKeys {
		t.Run("Upload_"+key, func(t *testing.T) {
			err := store.Upload(ctx, key, strings.NewReader("malicious content"), nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
			assert.Contains(t, err.Error(), "path traversal")
		})

		t.Run("Download_"+key, func(t *testing.T) {
			_, _, err := store.Download(ctx, key)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
		})

		t.Run("Delete_"+key, func(t *testing.T) {
			err := store.Delete(ctx, key)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
		})

		t.Run("Exists_"+key, func(t *testing.T) {
			_, err := store.Exists(ctx, key)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
		})

		t.Run("GetMetadata_"+key, func(t *testing.T) {
			_, err := store.GetMetadata(ctx, key)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
		})
	}
}

func TestStore_ValidateKey(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		// Valid keys.
		{name: "simple key", key: "test.tfplan", wantErr: false},
		{name: "nested key", key: "stack/component/sha.tfplan", wantErr: false},
		{name: "deeply nested", key: "a/b/c/d/e/f.tfplan", wantErr: false},
		// Invalid keys (path traversal attempts).
		{name: "parent escape", key: "../escape", wantErr: true},
		{name: "double parent escape", key: "../../etc/passwd", wantErr: true},
		{name: "hidden escape", key: "valid/../../../escape", wantErr: true},
		{name: "mid-path escape", key: "stack/../../../escape", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.validateKey(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrPlanfileKeyInvalid)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
