package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreationPreflightsAllOutputs(t *testing.T) {
	for _, paths := range [][]string{{"first.txt", "existing.txt"}, {"same.txt", "./same.txt"}, {"parent", "parent/child"}, {"../escape"}} {
		t.Run(paths[0], func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "existing.txt"), []byte("keep"), 0o644))
			var files []File
			for _, path := range paths {
				files = append(files, File{Path: path, Content: "new", Permissions: 0o644})
			}
			require.Error(t, ApplyCreation(context.Background(), root, files))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, "existing.txt", entries[0].Name())
			content, err := os.ReadFile(filepath.Join(root, "existing.txt"))
			require.NoError(t, err)
			assert.Equal(t, "keep", string(content))
		})
	}
}

func TestCreationWritesRenderedBytes(t *testing.T) {
	root := t.TempDir()
	files := []File{{Path: "a/file.yaml", Content: "vars: {}\n", Permissions: 0o640}, {Path: "b/file.txt", Content: "literal {{ untouched }}", Permissions: 0o644}}
	require.NoError(t, ApplyCreation(context.Background(), root, files))
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file.Path))
		require.NoError(t, err)
		assert.Equal(t, file.Content, string(content))
	}
	require.Error(t, ApplyCreation(context.Background(), root, files))
}
