package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestBundledStore_Name(t *testing.T) {
	ctrl := gomock.NewController(t)
	backend := NewMockBackend(ctrl)
	backend.EXPECT().Name().Return("test-backend")

	store := NewBundledStore(backend)
	assert.Equal(t, "test-backend", store.Name())
}

func TestBundledStore_Upload(t *testing.T) {
	t.Run("bundles files into tar and computes SHA256", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := NewMockBackend(ctrl)

		var capturedData []byte
		var capturedMetadata *Metadata
		backend.EXPECT().Upload(gomock.Any(), "my-artifact", gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, data io.Reader, _ int64, metadata *Metadata) error {
				var err error
				capturedData, err = io.ReadAll(data)
				require.NoError(t, err)
				capturedMetadata = metadata
				return nil
			})

		store := NewBundledStore(backend)
		files := []FileEntry{
			{Name: "plan.tfplan", Data: strings.NewReader("plan content"), Size: 12},
			{Name: "lock.hcl", Data: strings.NewReader("lock content"), Size: 12},
		}
		metadata := &Metadata{
			Stack:     "dev",
			Component: "vpc",
			CreatedAt: time.Now(),
		}

		err := store.Upload(context.Background(), "my-artifact", files, metadata)
		require.NoError(t, err)

		// Verify tar was created.
		require.NotEmpty(t, capturedData)

		// Verify SHA256 was computed and set.
		require.NotNil(t, capturedMetadata)
		assert.NotEmpty(t, capturedMetadata.SHA256)
		assert.Len(t, capturedMetadata.SHA256, 64) // SHA256 hex string.

		// Verify we can extract files from the tar.
		results, err := ExtractTarArchive(bytes.NewReader(capturedData))
		require.NoError(t, err)
		assert.Len(t, results, 2)

		fileMap := make(map[string]string)
		for _, r := range results {
			data, readErr := io.ReadAll(r.Data)
			require.NoError(t, readErr)
			fileMap[r.Name] = string(data)
		}
		assert.Equal(t, "plan content", fileMap["plan.tfplan"])
		assert.Equal(t, "lock content", fileMap["lock.hcl"])
	})

	t.Run("nil metadata creates default with SHA256", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := NewMockBackend(ctrl)

		var capturedMetadata *Metadata
		backend.EXPECT().Upload(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ io.Reader, _ int64, metadata *Metadata) error {
				capturedMetadata = metadata
				return nil
			})

		store := NewBundledStore(backend)
		files := []FileEntry{
			{Name: "file.txt", Data: strings.NewReader("hello"), Size: 5},
		}

		err := store.Upload(context.Background(), "test", files, nil)
		require.NoError(t, err)

		require.NotNil(t, capturedMetadata)
		assert.NotEmpty(t, capturedMetadata.SHA256)
		assert.False(t, capturedMetadata.CreatedAt.IsZero())
	})
}

func TestBundledStore_Download(t *testing.T) {
	t.Run("extracts files from tar stream", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := NewMockBackend(ctrl)

		// Create a tar archive to return from backend.
		files := []FileEntry{
			{Name: "plan.tfplan", Data: strings.NewReader("plan data"), Size: 9},
			{Name: "lock.hcl", Data: strings.NewReader("lock data"), Size: 9},
		}
		tarData, err := CreateTarArchive(files)
		require.NoError(t, err)

		metadata := &Metadata{Stack: "dev", Component: "vpc"}
		backend.EXPECT().Download(gomock.Any(), "my-artifact").
			Return(io.NopCloser(bytes.NewReader(tarData)), metadata, nil)

		store := NewBundledStore(backend)
		results, meta, err := store.Download(context.Background(), "my-artifact")
		require.NoError(t, err)
		assert.Len(t, results, 2)
		require.NotNil(t, meta)
		assert.Equal(t, "dev", meta.Stack)

		fileMap := make(map[string]string)
		for _, r := range results {
			data, readErr := io.ReadAll(r.Data)
			require.NoError(t, readErr)
			fileMap[r.Name] = string(data)
			r.Data.Close()
		}
		assert.Equal(t, "plan data", fileMap["plan.tfplan"])
		assert.Equal(t, "lock data", fileMap["lock.hcl"])
	})

	t.Run("SHA256 match succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := NewMockBackend(ctrl)

		files := []FileEntry{
			{Name: "plan.tfplan", Data: strings.NewReader("plan data"), Size: 9},
		}
		tarData, err := CreateTarArchive(files)
		require.NoError(t, err)

		// Compute correct SHA256.
		h := sha256.Sum256(tarData)
		sha256Hex := hex.EncodeToString(h[:])

		metadata := &Metadata{Stack: "dev", Component: "vpc"}
		metadata.SHA256 = sha256Hex
		backend.EXPECT().Download(gomock.Any(), "sha-match").
			Return(io.NopCloser(bytes.NewReader(tarData)), metadata, nil)

		store := NewBundledStore(backend)
		results, _, err := store.Download(context.Background(), "sha-match")
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("SHA256 mismatch returns integrity error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := NewMockBackend(ctrl)

		files := []FileEntry{
			{Name: "plan.tfplan", Data: strings.NewReader("plan data"), Size: 9},
		}
		tarData, err := CreateTarArchive(files)
		require.NoError(t, err)

		metadata := &Metadata{Stack: "dev", Component: "vpc"}
		metadata.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
		backend.EXPECT().Download(gomock.Any(), "sha-mismatch").
			Return(io.NopCloser(bytes.NewReader(tarData)), metadata, nil)

		store := NewBundledStore(backend)
		_, _, err = store.Download(context.Background(), "sha-mismatch")
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
	})

	t.Run("empty SHA256 skips verification", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		backend := NewMockBackend(ctrl)

		files := []FileEntry{
			{Name: "plan.tfplan", Data: strings.NewReader("plan data"), Size: 9},
		}
		tarData, err := CreateTarArchive(files)
		require.NoError(t, err)

		// Metadata with empty SHA256 — should skip verification.
		metadata := &Metadata{Stack: "dev", Component: "vpc"}
		backend.EXPECT().Download(gomock.Any(), "no-sha").
			Return(io.NopCloser(bytes.NewReader(tarData)), metadata, nil)

		store := NewBundledStore(backend)
		results, _, err := store.Download(context.Background(), "no-sha")
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})
}

func TestBundledStore_Passthrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	backend := NewMockBackend(ctrl)
	ctx := context.Background()

	t.Run("Delete delegates to backend", func(t *testing.T) {
		backend.EXPECT().Delete(ctx, "test-artifact").Return(nil)
		store := NewBundledStore(backend)
		err := store.Delete(ctx, "test-artifact")
		assert.NoError(t, err)
	})

	t.Run("List delegates to backend", func(t *testing.T) {
		expected := []ArtifactInfo{{Name: "a1"}, {Name: "a2"}}
		query := Query{Stacks: []string{"dev"}}
		backend.EXPECT().List(ctx, query).Return(expected, nil)
		store := NewBundledStore(backend)
		result, err := store.List(ctx, query)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Exists delegates to backend", func(t *testing.T) {
		backend.EXPECT().Exists(ctx, "test-artifact").Return(true, nil)
		store := NewBundledStore(backend)
		exists, err := store.Exists(ctx, "test-artifact")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("GetMetadata delegates to backend", func(t *testing.T) {
		expected := &Metadata{Stack: "dev", Component: "vpc"}
		backend.EXPECT().GetMetadata(ctx, "test-artifact").Return(expected, nil)
		store := NewBundledStore(backend)
		meta, err := store.GetMetadata(ctx, "test-artifact")
		assert.NoError(t, err)
		assert.Equal(t, expected, meta)
	})
}

func TestBundledStore_RoundTrip(t *testing.T) {
	// Test that upload → download preserves file content.
	ctrl := gomock.NewController(t)
	backend := NewMockBackend(ctrl)

	var storedData []byte
	var storedMetadata *Metadata

	backend.EXPECT().Upload(gomock.Any(), "roundtrip", gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, data io.Reader, _ int64, metadata *Metadata) error {
			var err error
			storedData, err = io.ReadAll(data)
			storedMetadata = metadata
			return err
		})

	backend.EXPECT().Download(gomock.Any(), "roundtrip").
		DoAndReturn(func(_ context.Context, _ string) (io.ReadCloser, *Metadata, error) {
			return io.NopCloser(bytes.NewReader(storedData)), storedMetadata, nil
		})

	store := NewBundledStore(backend)
	ctx := context.Background()

	// Upload.
	uploadFiles := []FileEntry{
		{Name: "a.txt", Data: strings.NewReader("hello"), Size: 5},
		{Name: "b.txt", Data: strings.NewReader("world"), Size: 5},
	}
	err := store.Upload(ctx, "roundtrip", uploadFiles, &Metadata{Stack: "test"})
	require.NoError(t, err)

	// Download.
	results, meta, err := store.Download(ctx, "roundtrip")
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "test", meta.Stack)
	assert.NotEmpty(t, meta.SHA256)

	fileMap := make(map[string]string)
	for _, r := range results {
		data, readErr := io.ReadAll(r.Data)
		require.NoError(t, readErr)
		fileMap[r.Name] = string(data)
		r.Data.Close()
	}
	assert.Equal(t, "hello", fileMap["a.txt"])
	assert.Equal(t, "world", fileMap["b.txt"])
}
