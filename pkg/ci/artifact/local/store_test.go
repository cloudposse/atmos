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

	name := "my-artifact"
	files := []artifact.FileEntry{
		{Name: "plan.tfplan", Data: strings.NewReader("plan content"), Size: 12},
		{Name: ".terraform.lock.hcl", Data: strings.NewReader("lock content"), Size: 12},
	}
	metadata := &artifact.Metadata{
		Stack:     "dev-us-east-1",
		Component: "vpc",
		SHA:       "abc123",
		CreatedAt: time.Now(),
	}

	// Upload.
	err := store.Upload(ctx, name, files, metadata)
	require.NoError(t, err)

	// Download.
	results, downloadedMeta, err := store.Download(ctx, name)
	require.NoError(t, err)
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	// Verify file count.
	assert.Len(t, results, 2)

	// Verify files.
	fileMap := make(map[string]string)
	for _, r := range results {
		data, err := io.ReadAll(r.Data)
		require.NoError(t, err)
		fileMap[r.Name] = string(data)
	}
	assert.Equal(t, "plan content", fileMap["plan.tfplan"])
	assert.Equal(t, "lock content", fileMap[".terraform.lock.hcl"])

	// Verify metadata.
	require.NotNil(t, downloadedMeta)
	assert.Equal(t, metadata.Stack, downloadedMeta.Stack)
	assert.Equal(t, metadata.Component, downloadedMeta.Component)
	assert.Equal(t, metadata.SHA, downloadedMeta.SHA)
	assert.NotEmpty(t, downloadedMeta.SHA256)
}

func TestStore_UploadSingleFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "single-file-artifact"
	files := []artifact.FileEntry{
		{Name: "output.txt", Data: strings.NewReader("hello"), Size: 5},
	}

	err := store.Upload(ctx, name, files, nil)
	require.NoError(t, err)

	results, meta, err := store.Download(ctx, name)
	require.NoError(t, err)
	defer func() {
		for _, r := range results {
			r.Data.Close()
		}
	}()

	assert.Len(t, results, 1)
	assert.Equal(t, "output.txt", results[0].Name)

	data, err := io.ReadAll(results[0].Data)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))

	// Metadata should be auto-created with SHA256.
	require.NotNil(t, meta)
	assert.NotEmpty(t, meta.SHA256)
}

func TestStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "to-delete"
	err := store.Upload(ctx, name, []artifact.FileEntry{
		{Name: "file.txt", Data: strings.NewReader("content"), Size: 7},
	}, nil)
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
	err := store.Delete(ctx, "nonexistent-artifact")
	assert.NoError(t, err)
}

func TestStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload artifacts with different metadata.
	artifacts := []struct {
		name     string
		metadata *artifact.Metadata
	}{
		{
			name: "artifact-1",
			metadata: &artifact.Metadata{
				Stack:     "dev",
				Component: "vpc",
				SHA:       "sha1",
				CreatedAt: time.Now(),
			},
		},
		{
			name: "artifact-2",
			metadata: &artifact.Metadata{
				Stack:     "staging",
				Component: "vpc",
				SHA:       "sha2",
				CreatedAt: time.Now(),
			},
		},
		{
			name: "artifact-3",
			metadata: &artifact.Metadata{
				Stack:     "dev",
				Component: "rds",
				SHA:       "sha1",
				CreatedAt: time.Now(),
			},
		},
	}

	for _, a := range artifacts {
		err := store.Upload(ctx, a.name, []artifact.FileEntry{
			{Name: "file.txt", Data: strings.NewReader("data"), Size: 4},
		}, a.metadata)
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
	assert.Equal(t, "artifact-2", list[0].Name)

	// Filter by stack AND component.
	list, err = store.List(ctx, artifact.Query{Stacks: []string{"dev"}, Components: []string{"vpc"}})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "artifact-1", list[0].Name)
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
	for _, name := range []string{"a1", "a2"} {
		err := store.Upload(ctx, name, []artifact.FileEntry{
			{Name: "f.txt", Data: strings.NewReader("d"), Size: 1},
		}, &artifact.Metadata{
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

	name := "exists-test"
	err := store.Upload(ctx, name, []artifact.FileEntry{
		{Name: "file.txt", Data: strings.NewReader("content"), Size: 7},
	}, nil)
	require.NoError(t, err)

	// Exists.
	exists, err := store.Exists(ctx, name)
	assert.NoError(t, err)
	assert.True(t, exists)

	// Not exists.
	exists, err = store.Exists(ctx, "nonexistent")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestStore_GetMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "meta-test"
	metadata := &artifact.Metadata{
		Stack:     "production",
		Component: "database",
		SHA:       "def456",
		Branch:    "main",
		PRNumber:  42,
		CreatedAt: time.Now(),
	}
	err := store.Upload(ctx, name, []artifact.FileEntry{
		{Name: "plan.bin", Data: strings.NewReader("binary data"), Size: 11},
	}, metadata)
	require.NoError(t, err)

	retrieved, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, metadata.Stack, retrieved.Stack)
	assert.Equal(t, metadata.Component, retrieved.Component)
	assert.Equal(t, metadata.SHA, retrieved.SHA)
	assert.Equal(t, metadata.Branch, retrieved.Branch)
	assert.Equal(t, metadata.PRNumber, retrieved.PRNumber)
	assert.NotEmpty(t, retrieved.SHA256)
}

func TestStore_GetMetadata_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	_, err := store.GetMetadata(ctx, "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
}

func TestStore_Download_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	_, _, err := store.Download(ctx, "nonexistent")
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
			err := store.Upload(ctx, name, []artifact.FileEntry{
				{Name: "file.txt", Data: strings.NewReader("data"), Size: 4},
			}, nil)
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
		{name: "simple name", input: "my-artifact", wantErr: false},
		{name: "nested name", input: "stack/component/sha", wantErr: false},
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

func TestStore_Upload_SHA256(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	name := "sha-test"
	files := []artifact.FileEntry{
		{Name: "a.txt", Data: strings.NewReader("hello"), Size: 5},
		{Name: "b.txt", Data: strings.NewReader("world"), Size: 5},
	}

	err := store.Upload(ctx, name, files, nil)
	require.NoError(t, err)

	meta, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.NotEmpty(t, meta.SHA256)
	// SHA256 should be a 64-char hex string.
	assert.Len(t, meta.SHA256, 64)
}

func TestStore_GetMetadata_NoSidecar(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Create artifact directory without metadata sidecar.
	name := "no-sidecar"
	artifactDir := filepath.Join(tmpDir, name)
	require.NoError(t, os.MkdirAll(artifactDir, defaultDirPerms))
	require.NoError(t, os.WriteFile(filepath.Join(artifactDir, "file.txt"), []byte("data"), defaultFilePerms))

	meta, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.NotNil(t, meta)
	// Should have CreatedAt from directory modtime.
	assert.False(t, meta.CreatedAt.IsZero())
}

func TestStore_Delete_CleansUpEmptyDirs(t *testing.T) {
	tmpDir := t.TempDir()
	store := &Store{basePath: tmpDir}
	ctx := context.Background()

	// Upload to a nested path.
	name := "nested/deep/artifact"
	err := store.Upload(ctx, name, []artifact.FileEntry{
		{Name: "file.txt", Data: strings.NewReader("content"), Size: 7},
	}, nil)
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

	err := store.Upload(ctx, "filtered", []artifact.FileEntry{
		{Name: "f.txt", Data: strings.NewReader("d"), Size: 1},
	}, &artifact.Metadata{
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

	store, err := NewStore(artifact.StoreOptions{
		Options: map[string]any{
			"path": tmpDir,
		},
	})
	require.NoError(t, err)

	ctx := context.Background()
	name := "lifecycle-test"
	metadata := &artifact.Metadata{
		Stack:     "production",
		Component: "vpc",
		SHA:       "abc123",
		CreatedAt: time.Now(),
	}
	files := []artifact.FileEntry{
		{Name: "plan.tfplan", Data: strings.NewReader("terraform plan"), Size: 14},
		{Name: "lock.hcl", Data: strings.NewReader("lock file"), Size: 9},
	}

	// 1. Upload.
	err = store.Upload(ctx, name, files, metadata)
	require.NoError(t, err)

	// 2. Exists.
	exists, err := store.Exists(ctx, name)
	require.NoError(t, err)
	assert.True(t, exists)

	// 3. GetMetadata.
	meta, err := store.GetMetadata(ctx, name)
	require.NoError(t, err)
	assert.Equal(t, "production", meta.Stack)
	assert.Equal(t, "vpc", meta.Component)
	assert.NotEmpty(t, meta.SHA256)

	// 4. List.
	list, err := store.List(ctx, artifact.Query{Components: []string{"vpc"}})
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, name, list[0].Name)

	// 5. Download.
	results, downloadedMeta, err := store.Download(ctx, name)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.NotNil(t, downloadedMeta)
	for _, r := range results {
		data, readErr := io.ReadAll(r.Data)
		require.NoError(t, readErr)
		assert.NotEmpty(t, data)
		r.Data.Close()
	}

	// 6. Delete.
	err = store.Delete(ctx, name)
	require.NoError(t, err)

	// 7. Verify deleted.
	exists, err = store.Exists(ctx, name)
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify metadata sidecar is also gone.
	_, err = os.Stat(filepath.Join(tmpDir, name+metadataSuffix))
	assert.True(t, os.IsNotExist(err))
}
