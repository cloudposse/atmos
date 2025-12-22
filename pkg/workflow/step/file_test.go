package step

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// FileHandler registration and basic validation are tested in interactive_handlers_test.go.
// This file tests helper methods.

func TestFileHandler_ResolveStartPath(t *testing.T) {
	handler, ok := Get("file")
	require.True(t, ok)
	fileHandler := handler.(*FileHandler)

	t.Run("default path is current directory", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Path: "",
		}
		vars := NewVariables()

		path, err := fileHandler.resolveStartPath(step, vars)
		require.NoError(t, err)
		// Should be an absolute path.
		assert.True(t, filepath.IsAbs(path))
	})

	t.Run("static path", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Path: "/tmp",
		}
		vars := NewVariables()

		path, err := fileHandler.resolveStartPath(step, vars)
		require.NoError(t, err)
		// On macOS, /tmp is symlinked to /private/tmp.
		assert.True(t, path == "/tmp" || path == "/private/tmp")
	})

	t.Run("template path", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Path: "{{ .steps.dir.value }}",
		}
		vars := NewVariables()
		vars.Set("dir", NewStepResult("/tmp"))

		path, err := fileHandler.resolveStartPath(step, vars)
		require.NoError(t, err)
		assert.True(t, path == "/tmp" || path == "/private/tmp")
	})

	t.Run("invalid template", func(t *testing.T) {
		step := &schema.WorkflowStep{
			Name: "test",
			Path: "{{ .steps.invalid.value",
		}
		vars := NewVariables()

		_, err := fileHandler.resolveStartPath(step, vars)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to resolve path")
	})
}

func TestFileHandler_MatchesExtensions(t *testing.T) {
	handler, ok := Get("file")
	require.True(t, ok)
	fileHandler := handler.(*FileHandler)

	tests := []struct {
		name       string
		path       string
		extensions []string
		expected   bool
	}{
		{
			name:       "empty extensions matches all",
			path:       "file.txt",
			extensions: []string{},
			expected:   true,
		},
		{
			name:       "nil extensions matches all",
			path:       "file.go",
			extensions: nil,
			expected:   true,
		},
		{
			name:       "matching extension with dot",
			path:       "file.go",
			extensions: []string{".go"},
			expected:   true,
		},
		{
			name:       "matching extension without dot",
			path:       "file.go",
			extensions: []string{"go"},
			expected:   true,
		},
		{
			name:       "non-matching extension",
			path:       "file.txt",
			extensions: []string{".go", ".py"},
			expected:   false,
		},
		{
			name:       "case insensitive match",
			path:       "file.GO",
			extensions: []string{".go"},
			expected:   true,
		},
		{
			name:       "multiple extensions with match",
			path:       "file.yaml",
			extensions: []string{".json", ".yaml", ".yml"},
			expected:   true,
		},
		{
			name:       "no extension matches any",
			path:       "Makefile",
			extensions: []string{".go", ".py"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fileHandler.matchesExtensions(tt.path, tt.extensions)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFileHandler_CollectFiles(t *testing.T) {
	handler, ok := Get("file")
	require.True(t, ok)
	fileHandler := handler.(*FileHandler)

	t.Run("collect files from directory", func(t *testing.T) {
		// Create temp directory with files.
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("test"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "subdir"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "subdir", "file3.txt"), []byte("test"), 0o644))

		step := &schema.WorkflowStep{
			Name:       "test",
			Extensions: []string{},
		}

		files, err := fileHandler.collectFiles(step, tmpDir)
		require.NoError(t, err)
		assert.Len(t, files, 3)
	})

	t.Run("collect files with extension filter", func(t *testing.T) {
		// Create temp directory with files.
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("test"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file3.txt"), []byte("test"), 0o644))

		step := &schema.WorkflowStep{
			Name:       "test",
			Extensions: []string{".txt"},
		}

		files, err := fileHandler.collectFiles(step, tmpDir)
		require.NoError(t, err)
		assert.Len(t, files, 2)
		// All files should be .txt.
		for _, f := range files {
			assert.Equal(t, ".txt", filepath.Ext(f))
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		step := &schema.WorkflowStep{
			Name:       "test",
			Extensions: []string{},
		}

		files, err := fileHandler.collectFiles(step, tmpDir)
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("handles subdirectory structure", func(t *testing.T) {
		// Create temp directory with nested structure.
		tmpDir := t.TempDir()
		subdir1 := filepath.Join(tmpDir, "dir1")
		subdir2 := filepath.Join(tmpDir, "dir2", "nested")
		require.NoError(t, os.MkdirAll(subdir1, 0o755))
		require.NoError(t, os.MkdirAll(subdir2, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.go"), []byte("test"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(subdir1, "level1.go"), []byte("test"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(subdir2, "level2.go"), []byte("test"), 0o644))

		step := &schema.WorkflowStep{
			Name:       "test",
			Extensions: []string{".go"},
		}

		files, err := fileHandler.collectFiles(step, tmpDir)
		require.NoError(t, err)
		assert.Len(t, files, 3)

		// Verify paths are relative.
		for _, f := range files {
			assert.False(t, filepath.IsAbs(f), "paths should be relative: %s", f)
		}
	})

	t.Run("skips directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		subdir := filepath.Join(tmpDir, "subdir.txt") // Directory named with extension.
		require.NoError(t, os.MkdirAll(subdir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0o644))

		step := &schema.WorkflowStep{
			Name:       "test",
			Extensions: []string{".txt"},
		}

		files, err := fileHandler.collectFiles(step, tmpDir)
		require.NoError(t, err)
		// Should only have the file, not the directory.
		assert.Len(t, files, 1)
		assert.Equal(t, "file.txt", files[0])
	})
}
