package local

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/artifact"
)

func TestNewStore(t *testing.T) {
	tests := []struct {
		name        string
		opts        artifact.StoreOptions
		expectError bool
	}{
		{
			name: "valid options",
			opts: artifact.StoreOptions{
				Options: map[string]any{
					"path": t.TempDir(),
				},
			},
			expectError: false,
		},
		{
			name: "default path",
			opts: artifact.StoreOptions{
				Options: map[string]any{},
			},
			expectError: false,
		},
		{
			name: "tilde expansion",
			opts: artifact.StoreOptions{
				Options: map[string]any{
					"path": "~/.atmos-test-artifacts",
				},
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
				// Clean up tilde-expanded dirs.
				if tt.name == "tilde expansion" {
					localStore := store.(*Store)
					_ = os.RemoveAll(localStore.basePath)
				}
			}
		})
	}

	// Clean up default path.
	_ = os.RemoveAll(".atmos/artifacts")
}

func TestStore_Name(t *testing.T) {
	store := &Store{basePath: "/tmp"}
	assert.Equal(t, "local", store.Name())
}

func TestStore_UploadDownload(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "my-artifact.tar"
	data := "tar archive content here"
	metadata := &artifact.Metadata{
		Stack:     "dev-us-east-1",
		Component: "vpc",
		SHA:       "abc123",
		SHA256:    "precomputed-sha256",
		CreatedAt: time.Now(),
	}

	// Upload.
	err := store.Upload(ctx, name, strings.NewReader(data), int64(len(data)), metadata)
	require.NoError(t, err)

	// Download.
	reader, downloadedMeta, err := store.Download(ctx, name)
	require.NoError(t, err)
	defer reader.Close()

	// Verify data.
	downloaded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, data, string(downloaded))

	// Verify metadata.
	require.NotNil(t, downloadedMeta)
	assert.Equal(t, metadata.Stack, downloadedMeta.Stack)
	assert.Equal(t, metadata.Component, downloadedMeta.Component)
	assert.Equal(t, metadata.SHA, downloadedMeta.SHA)
	assert.Equal(t, metadata.SHA256, downloadedMeta.SHA256)
}

func TestStore_UploadNilMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "nil-meta-artifact.tar"
	data := "some data"

	err := store.Upload(ctx, name, strings.NewReader(data), int64(len(data)), nil)
	require.NoError(t, err)

	// Metadata should be auto-created.
	meta, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.False(t, meta.CreatedAt.IsZero())
}

func TestStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "to-delete.tar"
	err := store.Upload(ctx, name, strings.NewReader("content"), 7, nil)
	require.NoError(t, err)

	// Verify it exists.
	exists, err := store.Exists(ctx, name)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete.
	err = store.Delete(ctx, name)
	assert.NoError(t, err)

	// Verify it's gone.
	exists, err = store.Exists(ctx, name)
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify metadata sidecar is also gone.
	_, err = os.Stat(filepath.Join(tmpDir, name+metadataSuffix))
	assert.True(t, os.IsNotExist(err))
}

func TestStore_Delete_NotExists(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Delete should be idempotent.
	err := store.Delete(ctx, "nonexistent-artifact.tar")
	assert.NoError(t, err)
}

func TestStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload artifacts with different metadata.
	testArtifacts := []struct {
		name     string
		metadata *artifact.Metadata
	}{
		{
			name: "artifact-1.tar",
			metadata: &artifact.Metadata{
				Stack:     "dev",
				Component: "vpc",
				SHA:       "sha1",
				CreatedAt: time.Now(),
			},
		},
		{
			name: "artifact-2.tar",
			metadata: &artifact.Metadata{
				Stack:     "staging",
				Component: "vpc",
				SHA:       "sha2",
				CreatedAt: time.Now(),
			},
		},
		{
			name: "artifact-3.tar",
			metadata: &artifact.Metadata{
				Stack:     "dev",
				Component: "rds",
				SHA:       "sha1",
				CreatedAt: time.Now(),
			},
		},
	}

	for _, a := range testArtifacts {
		err := store.Upload(ctx, a.name, strings.NewReader("data"), 4, a.metadata)
		require.NoError(t, err)
	}

	// List all with empty query (no filters = match all).
	list, err := store.List(ctx, artifact.Query{})
	require.NoError(t, err)
	assert.Len(t, list, 3)

	// Filter by component.
	list, err = store.List(ctx, artifact.Query{Components: []string{"vpc"}})
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// Filter by stack.
	list, err = store.List(ctx, artifact.Query{Stacks: []string{"dev"}})
	require.NoError(t, err)
	assert.Len(t, list, 2)

	// Filter by SHA.
	list, err = store.List(ctx, artifact.Query{SHAs: []string{"sha2"}})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "artifact-2.tar", list[0].Name)

	// Filter by stack AND component.
	list, err = store.List(ctx, artifact.Query{Stacks: []string{"dev"}, Components: []string{"vpc"}})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "artifact-1.tar", list[0].Name)
}

func TestStore_List_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	list, err := store.List(ctx, artifact.Query{})
	assert.NoError(t, err)
	assert.Empty(t, list)
}

func TestStore_List_AllFlag(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload artifacts with metadata.
	for _, name := range []string{"a1.tar", "a2.tar"} {
		err := store.Upload(ctx, name, strings.NewReader("d"), 1, &artifact.Metadata{
			Stack:     "dev",
			Component: "vpc",
			SHA:       "sha1",
			CreatedAt: time.Now(),
		})
		require.NoError(t, err)
	}

	// Query.All should return everything regardless of other filters.
	list, err := store.List(ctx, artifact.Query{
		All:        true,
		Components: []string{"nonexistent"},
	})
	require.NoError(t, err)
	assert.Len(t, list, 2)
}

func TestStore_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "exists-test.tar"
	err := store.Upload(ctx, name, strings.NewReader("content"), 7, nil)
	require.NoError(t, err)

	// Exists.
	exists, err := store.Exists(ctx, name)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Not exists.
	exists, err = store.Exists(ctx, "nonexistent.tar")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_GetMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "meta-test.tar"
	metadata := &artifact.Metadata{
		Stack:     "production",
		Component: "database",
		SHA:       "def456",
		SHA256:    "abc123sha256",
		Branch:    "main",
		PRNumber:  42,
		CreatedAt: time.Now(),
	}
	err := store.Upload(ctx, name, strings.NewReader("binary data"), 11, metadata)
	require.NoError(t, err)

	retrieved, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, metadata.Stack, retrieved.Stack)
	assert.Equal(t, metadata.Component, retrieved.Component)
	assert.Equal(t, metadata.SHA, retrieved.SHA)
	assert.Equal(t, metadata.Branch, retrieved.Branch)
	assert.Equal(t, metadata.PRNumber, retrieved.PRNumber)
	assert.Equal(t, metadata.SHA256, retrieved.SHA256)
}

func TestStore_GetMetadata_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	_, err := store.GetMetadata(ctx, "nonexistent.tar")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestStore_Download_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	_, _, err := store.Download(ctx, "nonexistent.tar")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestStore_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	maliciousNames := []string{
		"../escape",
		"../../etc/passwd",
		"valid/../../../escape",
		"artifact/../../../etc/passwd",
	}

	for _, name := range maliciousNames {
		t.Run("Upload_"+name, func(t *testing.T) {
			err := store.Upload(ctx, name, strings.NewReader("data"), 4, nil)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactStoreInvalidArgs)
			assert.Contains(t, err.Error(), "path traversal")
		})

		t.Run("Download_"+name, func(t *testing.T) {
			_, _, err := store.Download(ctx, name)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactStoreInvalidArgs)
		})

		t.Run("Delete_"+name, func(t *testing.T) {
			err := store.Delete(ctx, name)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactStoreInvalidArgs)
		})

		t.Run("Exists_"+name, func(t *testing.T) {
			_, err := store.Exists(ctx, name)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactStoreInvalidArgs)
		})

		t.Run("GetMetadata_"+name, func(t *testing.T) {
			_, err := store.GetMetadata(ctx, name)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactStoreInvalidArgs)
		})
	}
}

func TestStore_ValidateName(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple name", input: "my-artifact.tar", wantErr: false},
		{name: "nested name", input: "stack/component/sha.tar", wantErr: false},
		{name: "deeply nested", input: "a/b/c/d/e", wantErr: false},
		{name: "with dots", input: "my.artifact.v1", wantErr: false},
		{name: "parent escape", input: "../escape", wantErr: true},
		{name: "double parent escape", input: "../../etc/passwd", wantErr: true},
		{name: "hidden escape", input: "valid/../../../escape", wantErr: true},
		{name: "mid-path escape", input: "stack/../../../escape", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.validateName(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrArtifactStoreInvalidArgs)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStore_GetMetadata_NoSidecar(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Create artifact file without metadata sidecar.
	name := "no-sidecar.tar"
	filePath := filepath.Join(tmpDir, name)
	require.NoError(t, os.WriteFile(filePath, []byte("data"), defaultFilePerms))

	meta, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.NotNil(t, meta)
	// Should have CreatedAt from file modtime.
	assert.False(t, meta.CreatedAt.IsZero())
}

func TestStore_Delete_CleansUpEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload to a nested path.
	name := "nested/deep/artifact.tar"
	err := store.Upload(ctx, name, strings.NewReader("content"), 7, nil)
	require.NoError(t, err)

	// Delete.
	err = store.Delete(ctx, name)
	require.NoError(t, err)

	// Empty parent dirs should be cleaned up.
	_, err = os.Stat(filepath.Join(tmpDir, "nested"))
	assert.True(t, os.IsNotExist(err), "expected nested dir to be removed")
}

func TestStore_List_NoMatchingFilters(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	err := store.Upload(ctx, "filtered.tar", strings.NewReader("d"), 1, &artifact.Metadata{
		Stack:     "dev",
		Component: "vpc",
		SHA:       "sha1",
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	// Query with non-matching filters.
	list, err := store.List(ctx, artifact.Query{Components: []string{"nonexistent"}})
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestIntegration_FullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	backend, err := NewStore(artifact.StoreOptions{
		Options: map[string]any{
			"path": tmpDir,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	name := "lifecycle-test.tar"
	metadata := &artifact.Metadata{
		Stack:     "production",
		Component: "vpc",
		SHA:       "abc123",
		SHA256:    "precomputed",
		CreatedAt: time.Now(),
	}
	data := "terraform plan data"

	// 1. Upload.
	err = backend.Upload(ctx, name, strings.NewReader(data), int64(len(data)), metadata)
	require.NoError(t, err)

	// 2. Exists.
	exists, err := backend.Exists(ctx, name)
	require.NoError(t, err)
	assert.True(t, exists)

	// 3. GetMetadata.
	meta, err := backend.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, "production", meta.Stack)
	assert.Equal(t, "vpc", meta.Component)
	assert.Equal(t, "precomputed", meta.SHA256)

	// 4. List.
	list, err := backend.List(ctx, artifact.Query{Components: []string{"vpc"}})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, name, list[0].Name)

	// 5. Download.
	reader, downloadedMeta, err := backend.Download(ctx, name)
	require.NoError(t, err)
	assert.NotNil(t, downloadedMeta)
	downloaded, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)
	assert.Equal(t, data, string(downloaded))
	reader.Close()

	// 6. Delete.
	err = backend.Delete(ctx, name)
	require.NoError(t, err)

	// 7. Verify deleted.
	exists, err = backend.Exists(ctx, name)
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify metadata sidecar is also gone.
	_, err = os.Stat(filepath.Join(tmpDir, name+metadataSuffix))
	assert.True(t, os.IsNotExist(err))
}
