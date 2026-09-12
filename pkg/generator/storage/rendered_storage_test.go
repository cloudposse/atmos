package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestRenderedBaseStorage_LoadBase(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("name: demo\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nested", "file.txt"), []byte("nested content\n"), 0o644))

	tests := []struct {
		name        string
		filePath    string
		wantContent string
		wantFound   bool
		wantErr     bool
	}{
		{
			name:        "existing top-level file",
			filePath:    "config.yaml",
			wantContent: "name: demo\n",
			wantFound:   true,
		},
		{
			name:        "existing nested file",
			filePath:    "nested/file.txt",
			wantContent: "nested content\n",
			wantFound:   true,
		},
		{
			name:      "missing file is not an error",
			filePath:  "does-not-exist.yaml",
			wantFound: false,
		},
		{
			name:      "path is cleaned before joining",
			filePath:  "./nested/../config.yaml",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewRenderedBaseStorage(root)

			content, found, err := storage.LoadBase(tt.filePath)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound && tt.wantContent != "" {
				assert.Equal(t, tt.wantContent, content)
			}
		})
	}
}

func TestRenderedBaseStorage_LoadBase_RootUnreadable(t *testing.T) {
	// A root that isn't a directory at all (e.g. a plain file in its place)
	// surfaces as a real read error, not a "not found" result -- distinct
	// from a missing individual file within a valid root.
	root := t.TempDir()
	blockingFile := filepath.Join(root, "blocking")
	require.NoError(t, os.WriteFile(blockingFile, []byte("x"), 0o644))

	storage := NewRenderedBaseStorage(blockingFile)

	_, found, err := storage.LoadBase("config.yaml")

	require.Error(t, err)
	assert.False(t, found)
	assert.ErrorIs(t, err, errUtils.ErrReadFile)
}
