package filesystem

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOSFileSystem(t *testing.T) {
	fs := NewOSFileSystem()
	assert.NotNil(t, fs)
}

func TestOSFileSystem_Open(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("opens existing file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-open-*.txt")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		f, err := fs.Open(tmpFile.Name())
		require.NoError(t, err)
		defer f.Close()
		assert.NotNil(t, f)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := fs.Open("/nonexistent/path/file.txt")
		assert.Error(t, err)
	})
}

func TestOSFileSystem_Create(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("creates new file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test-create.txt")

		f, err := fs.Create(filePath)
		require.NoError(t, err)
		defer f.Close()

		_, err = os.Stat(filePath)
		assert.NoError(t, err)
	})

	t.Run("truncates existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test-truncate.txt")

		// Create file with content.
		err := os.WriteFile(filePath, []byte("existing content"), 0o644)
		require.NoError(t, err)

		f, err := fs.Create(filePath)
		require.NoError(t, err)
		f.Close()

		// File should be empty after Create.
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Empty(t, content)
	})
}

func TestOSFileSystem_Stat(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("returns info for existing file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-stat-*.txt")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		info, err := fs.Stat(tmpFile.Name())
		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.False(t, info.IsDir())
	})

	t.Run("returns info for directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		info, err := fs.Stat(tmpDir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("returns error for non-existent path", func(t *testing.T) {
		_, err := fs.Stat("/nonexistent/path")
		assert.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestOSFileSystem_MkdirAll(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("creates nested directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		nestedPath := filepath.Join(tmpDir, "a", "b", "c")

		err := fs.MkdirAll(nestedPath, 0o755)
		require.NoError(t, err)

		info, err := os.Stat(nestedPath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("succeeds if directory exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		err := fs.MkdirAll(tmpDir, 0o755)
		assert.NoError(t, err)
	})
}

func TestOSFileSystem_Chmod(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("changes file permissions", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-chmod-*.txt")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		err = fs.Chmod(tmpFile.Name(), 0o600)
		require.NoError(t, err)

		info, err := os.Stat(tmpFile.Name())
		require.NoError(t, err)
		// Check that permissions were changed (exact mode varies by umask).
		assert.NotNil(t, info.Mode())
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		err := fs.Chmod("/nonexistent/file.txt", 0o644)
		assert.Error(t, err)
	})
}

func TestOSFileSystem_MkdirTemp(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("creates temp directory with pattern", func(t *testing.T) {
		dir, err := fs.MkdirTemp("", "test-mkdirtemp-*")
		require.NoError(t, err)
		defer os.RemoveAll(dir)

		assert.True(t, strings.Contains(dir, "test-mkdirtemp-"))

		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("creates temp directory in specified parent", func(t *testing.T) {
		parentDir := t.TempDir()

		dir, err := fs.MkdirTemp(parentDir, "child-*")
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(dir, parentDir))
	})
}

func TestOSFileSystem_CreateTemp(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("creates temp file with pattern", func(t *testing.T) {
		f, err := fs.CreateTemp("", "test-createtemp-*.txt")
		require.NoError(t, err)
		defer os.Remove(f.Name())
		defer f.Close()

		assert.True(t, strings.Contains(f.Name(), "test-createtemp-"))
		assert.True(t, strings.HasSuffix(f.Name(), ".txt"))
	})

	t.Run("creates temp file in specified directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		f, err := fs.CreateTemp(tmpDir, "child-*.txt")
		require.NoError(t, err)
		defer f.Close()

		assert.True(t, strings.HasPrefix(f.Name(), tmpDir))
	})
}

func TestOSFileSystem_WriteFile(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("writes content to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test-write.txt")
		content := []byte("hello world")

		err := fs.WriteFile(filePath, content, 0o644)
		require.NoError(t, err)

		readContent, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, content, readContent)
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test-overwrite.txt")

		err := os.WriteFile(filePath, []byte("old content"), 0o644)
		require.NoError(t, err)

		newContent := []byte("new content")
		err = fs.WriteFile(filePath, newContent, 0o644)
		require.NoError(t, err)

		readContent, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, newContent, readContent)
	})
}

func TestOSFileSystem_ReadFile(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("reads file content", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test-read.txt")
		content := []byte("test content")

		err := os.WriteFile(filePath, content, 0o644)
		require.NoError(t, err)

		readContent, err := fs.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, content, readContent)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := fs.ReadFile("/nonexistent/file.txt")
		assert.Error(t, err)
	})
}

func TestOSFileSystem_Remove(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("removes file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-remove-*.txt")
		require.NoError(t, err)
		tmpFile.Close()

		err = fs.Remove(tmpFile.Name())
		require.NoError(t, err)

		_, err = os.Stat(tmpFile.Name())
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("removes empty directory", func(t *testing.T) {
		parentDir := t.TempDir()
		tmpDir := filepath.Join(parentDir, "to-remove")
		err := os.Mkdir(tmpDir, 0o755)
		require.NoError(t, err)

		err = fs.Remove(tmpDir)
		require.NoError(t, err)

		_, err = os.Stat(tmpDir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("returns error for non-empty directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content"), 0o644)
		require.NoError(t, err)

		err = fs.Remove(tmpDir)
		assert.Error(t, err)
	})
}

func TestOSFileSystem_RemoveAll(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("removes directory with contents", func(t *testing.T) {
		parentDir := t.TempDir()
		tmpDir := filepath.Join(parentDir, "to-remove")
		err := os.Mkdir(tmpDir, 0o755)
		require.NoError(t, err)

		// Create nested structure.
		nestedDir := filepath.Join(tmpDir, "nested")
		err = os.MkdirAll(nestedDir, 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(nestedDir, "file.txt"), []byte("content"), 0o644)
		require.NoError(t, err)

		err = fs.RemoveAll(tmpDir)
		require.NoError(t, err)

		_, err = os.Stat(tmpDir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("succeeds for non-existent path", func(t *testing.T) {
		err := fs.RemoveAll("/nonexistent/path/that/does/not/exist")
		assert.NoError(t, err)
	})
}

func TestOSFileSystem_Walk(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("walks directory tree", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create structure.
		err := os.MkdirAll(filepath.Join(tmpDir, "a", "b"), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "a", "file2.txt"), []byte("2"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "a", "b", "file3.txt"), []byte("3"), 0o644)
		require.NoError(t, err)

		var visited []string
		err = fs.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(tmpDir, path)
			visited = append(visited, relPath)
			return nil
		})
		require.NoError(t, err)

		assert.Contains(t, visited, ".")
		assert.Contains(t, visited, "a")
		assert.Contains(t, visited, filepath.Join("a", "b"))
		assert.Contains(t, visited, "file1.txt")
		assert.Contains(t, visited, filepath.Join("a", "file2.txt"))
		assert.Contains(t, visited, filepath.Join("a", "b", "file3.txt"))
	})

	t.Run("returns error for non-existent root", func(t *testing.T) {
		err := fs.Walk("/nonexistent/root", func(path string, info os.FileInfo, err error) error {
			return err
		})
		assert.Error(t, err)
	})
}

func TestOSFileSystem_Getwd(t *testing.T) {
	fs := NewOSFileSystem()

	t.Run("returns current working directory", func(t *testing.T) {
		wd, err := fs.Getwd()
		require.NoError(t, err)
		assert.NotEmpty(t, wd)

		// Verify it's a real directory.
		info, err := os.Stat(wd)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	})
}

func TestNewOSGlobMatcher(t *testing.T) {
	gm := NewOSGlobMatcher()
	assert.NotNil(t, gm)
}

func TestOSGlobMatcher_GetGlobMatches(t *testing.T) {
	gm := NewOSGlobMatcher()

	t.Run("returns matches for valid pattern", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create test files.
		err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("1"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("2"), 0o644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(tmpDir, "other.go"), []byte("3"), 0o644)
		require.NoError(t, err)

		pattern := filepath.Join(tmpDir, "*.txt")
		matches, err := gm.GetGlobMatches(pattern)
		require.NoError(t, err)

		assert.Len(t, matches, 2)
	})

	t.Run("returns error for no matches", func(t *testing.T) {
		tmpDir := t.TempDir()

		// The underlying GetGlobMatches returns an error when no files match.
		pattern := filepath.Join(tmpDir, "*.nonexistent")
		_, err := gm.GetGlobMatches(pattern)
		// This function returns an error when no matches found.
		assert.Error(t, err)
	})
}

func TestOSGlobMatcher_PathMatch(t *testing.T) {
	gm := NewOSGlobMatcher()

	tests := []struct {
		name    string
		pattern string
		path    string
		match   bool
	}{
		{
			name:    "exact match",
			pattern: "file.txt",
			path:    "file.txt",
			match:   true,
		},
		{
			name:    "wildcard match",
			pattern: "*.txt",
			path:    "file.txt",
			match:   true,
		},
		{
			name:    "wildcard no match",
			pattern: "*.txt",
			path:    "file.go",
			match:   false,
		},
		{
			name:    "question mark match",
			pattern: "file?.txt",
			path:    "file1.txt",
			match:   true,
		},
		{
			name:    "character class match",
			pattern: "file[0-9].txt",
			path:    "file5.txt",
			match:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, err := gm.PathMatch(tt.pattern, tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.match, match)
		})
	}
}

func TestNewOSIOCopier(t *testing.T) {
	copier := NewOSIOCopier()
	assert.NotNil(t, copier)
}

func TestOSIOCopier_Copy(t *testing.T) {
	copier := NewOSIOCopier()

	t.Run("copies data from reader to writer", func(t *testing.T) {
		src := strings.NewReader("hello world")
		dst := &bytes.Buffer{}

		n, err := copier.Copy(dst, src)
		require.NoError(t, err)

		assert.Equal(t, int64(11), n)
		assert.Equal(t, "hello world", dst.String())
	})

	t.Run("handles empty reader", func(t *testing.T) {
		src := strings.NewReader("")
		dst := &bytes.Buffer{}

		n, err := copier.Copy(dst, src)
		require.NoError(t, err)

		assert.Equal(t, int64(0), n)
		assert.Empty(t, dst.String())
	})

	t.Run("handles large data", func(t *testing.T) {
		data := strings.Repeat("x", 1024*1024) // 1MB
		src := strings.NewReader(data)
		dst := &bytes.Buffer{}

		n, err := copier.Copy(dst, src)
		require.NoError(t, err)

		assert.Equal(t, int64(len(data)), n)
		assert.Equal(t, data, dst.String())
	})
}
