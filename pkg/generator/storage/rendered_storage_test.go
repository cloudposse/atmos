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

// TestRenderedBaseStorage_LoadBase_RejectsPathTraversal is a regression test
// for CWE-22: filepath.Clean alone does not strip a leading ".." (it only
// collapses redundant segments), so a filePath like "../../etc/passwd" used
// to walk filepath.Join(s.root, cleanPath) outside s.root entirely and read
// whatever file sat there via os.ReadFile. This can be reached without a
// caller bug: engine.Processor.determineBaseContent falls back to the
// scaffold's raw, un-rendered file.Path when filepath.Rel fails, which
// bypasses ProcessFile's own write-path validation (validateRenderedPath/
// validateWriteTarget) entirely, since that validation only ever gates
// writes, not this base-content read.
func TestRenderedBaseStorage_LoadBase_RejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte("name: demo\n"), 0o644))

	// A secret sitting just outside root, at the same level root's parent
	// directory would resolve to -- proving the pre-fix code could actually
	// reach outside root, not merely that a relative path fails to resolve.
	outsideDir := filepath.Dir(root)
	secretPath := filepath.Join(outsideDir, "secret.txt")
	require.NoError(t, os.WriteFile(secretPath, []byte("outside content\n"), 0o644))
	t.Cleanup(func() { _ = os.Remove(secretPath) })

	tests := []struct {
		name     string
		filePath string
	}{
		{name: "simple parent traversal", filePath: "../secret.txt"},
		{name: "traversal after a real segment", filePath: "nested/../../secret.txt"},
		{name: "traversal-only path", filePath: ".."},
		{name: "absolute path", filePath: "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := NewRenderedBaseStorage(root)

			content, found, err := storage.LoadBase(tt.filePath)

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
			assert.False(t, found)
			assert.Empty(t, content)
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
