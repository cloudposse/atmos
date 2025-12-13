package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestResolveWorkingDirectory tests the resolveWorkingDirectory function.
func TestResolveWorkingDirectory(t *testing.T) {
	tests := []struct {
		name        string
		workDir     string
		basePath    string
		defaultDir  string
		setup       func(t *testing.T) (workDir, basePath, defaultDir string)
		expected    func(t *testing.T, workDir, basePath, defaultDir string) string
		expectedErr error
	}{
		{
			name: "empty working directory returns default",
			setup: func(t *testing.T) (string, string, string) {
				defaultDir := t.TempDir()
				return "", "/some/base", defaultDir
			},
			expected: func(t *testing.T, workDir, basePath, defaultDir string) string {
				return defaultDir
			},
			expectedErr: nil,
		},
		{
			name: "absolute path to valid directory",
			setup: func(t *testing.T) (string, string, string) {
				absDir := t.TempDir()
				return absDir, "/some/base", "/default"
			},
			expected: func(t *testing.T, workDir, basePath, defaultDir string) string {
				return workDir
			},
			expectedErr: nil,
		},
		{
			name: "relative path resolved against basePath",
			setup: func(t *testing.T) (string, string, string) {
				baseDir := t.TempDir()
				relDir := "subdir"
				// Create the subdirectory.
				fullPath := filepath.Join(baseDir, relDir)
				err := os.MkdirAll(fullPath, 0o755)
				require.NoError(t, err)
				return relDir, baseDir, "/default"
			},
			expected: func(t *testing.T, workDir, basePath, defaultDir string) string {
				return filepath.Join(basePath, workDir)
			},
			expectedErr: nil,
		},
		{
			name: "nested relative path resolved against basePath",
			setup: func(t *testing.T) (string, string, string) {
				baseDir := t.TempDir()
				relDir := filepath.Join("nested", "subdir")
				// Create the nested subdirectory.
				fullPath := filepath.Join(baseDir, relDir)
				err := os.MkdirAll(fullPath, 0o755)
				require.NoError(t, err)
				return relDir, baseDir, "/default"
			},
			expected: func(t *testing.T, workDir, basePath, defaultDir string) string {
				return filepath.Join(basePath, workDir)
			},
			expectedErr: nil,
		},
		{
			name: "directory does not exist",
			setup: func(t *testing.T) (string, string, string) {
				return "/nonexistent/path/that/does/not/exist", "/base", "/default"
			},
			expected:    nil,
			expectedErr: errUtils.ErrWorkingDirNotFound,
		},
		{
			name: "relative path to nonexistent directory",
			setup: func(t *testing.T) (string, string, string) {
				baseDir := t.TempDir()
				return "nonexistent_subdir", baseDir, "/default"
			},
			expected:    nil,
			expectedErr: errUtils.ErrWorkingDirNotFound,
		},
		{
			name: "path is a file not a directory",
			setup: func(t *testing.T) (string, string, string) {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "testfile.txt")
				err := os.WriteFile(filePath, []byte("test content"), 0o644)
				require.NoError(t, err)
				return filePath, "/base", "/default"
			},
			expected:    nil,
			expectedErr: errUtils.ErrWorkingDirNotDirectory,
		},
		{
			name: "relative path to file not directory",
			setup: func(t *testing.T) (string, string, string) {
				baseDir := t.TempDir()
				fileName := "testfile.txt"
				filePath := filepath.Join(baseDir, fileName)
				err := os.WriteFile(filePath, []byte("test content"), 0o644)
				require.NoError(t, err)
				return fileName, baseDir, "/default"
			},
			expected:    nil,
			expectedErr: errUtils.ErrWorkingDirNotDirectory,
		},
		{
			name: "empty basePath with relative workDir",
			setup: func(t *testing.T) (string, string, string) {
				// When basePath is empty, relative paths are resolved against current directory.
				// Create a temp dir and change to it.
				tmpDir := t.TempDir()
				subDir := "subdir"
				fullPath := filepath.Join(tmpDir, subDir)
				err := os.MkdirAll(fullPath, 0o755)
				require.NoError(t, err)
				// Change to tmpDir so relative path works.
				t.Chdir(tmpDir)
				return subDir, "", "/default"
			},
			expected: func(t *testing.T, workDir, basePath, defaultDir string) string {
				// With empty basePath, filepath.Join("", "subdir") = "subdir".
				return workDir
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workDir, basePath, defaultDir := tt.setup(t)

			result, err := resolveWorkingDirectory(workDir, basePath, defaultDir)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
				assert.Empty(t, result)
			} else {
				require.NoError(t, err)
				expectedPath := tt.expected(t, workDir, basePath, defaultDir)
				assert.Equal(t, expectedPath, result)
			}
		})
	}
}

// TestResolveWorkingDirectory_EdgeCases tests edge cases for resolveWorkingDirectory.
func TestResolveWorkingDirectory_EdgeCases(t *testing.T) {
	t.Run("dot path resolves to basePath", func(t *testing.T) {
		baseDir := t.TempDir()

		result, err := resolveWorkingDirectory(".", baseDir, "/default")

		require.NoError(t, err)
		assert.Equal(t, baseDir, result)
	})

	t.Run("double dot path resolves to parent of basePath", func(t *testing.T) {
		// Create nested structure.
		tmpDir := t.TempDir()
		childDir := filepath.Join(tmpDir, "child")
		err := os.MkdirAll(childDir, 0o755)
		require.NoError(t, err)

		result, err := resolveWorkingDirectory("..", childDir, "/default")

		require.NoError(t, err)
		assert.Equal(t, tmpDir, result)
	})

	t.Run("whitespace in path is preserved", func(t *testing.T) {
		tmpDir := t.TempDir()
		dirWithSpaces := filepath.Join(tmpDir, "dir with spaces")
		err := os.MkdirAll(dirWithSpaces, 0o755)
		require.NoError(t, err)

		result, err := resolveWorkingDirectory(dirWithSpaces, "/base", "/default")

		require.NoError(t, err)
		assert.Equal(t, dirWithSpaces, result)
	})

	t.Run("symlink to directory is valid", func(t *testing.T) {
		tmpDir := t.TempDir()
		realDir := filepath.Join(tmpDir, "realdir")
		err := os.MkdirAll(realDir, 0o755)
		require.NoError(t, err)

		symlinkPath := filepath.Join(tmpDir, "symlink")
		err = os.Symlink(realDir, symlinkPath)
		if err != nil {
			t.Skipf("Skipping symlink test: %v", err)
		}

		result, err := resolveWorkingDirectory(symlinkPath, "/base", "/default")

		require.NoError(t, err)
		assert.Equal(t, symlinkPath, result)
	})

	t.Run("symlink to file is invalid", func(t *testing.T) {
		tmpDir := t.TempDir()
		realFile := filepath.Join(tmpDir, "realfile.txt")
		err := os.WriteFile(realFile, []byte("content"), 0o644)
		require.NoError(t, err)

		symlinkPath := filepath.Join(tmpDir, "symlink")
		err = os.Symlink(realFile, symlinkPath)
		if err != nil {
			t.Skipf("Skipping symlink test: %v", err)
		}

		_, err = resolveWorkingDirectory(symlinkPath, "/base", "/default")

		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrWorkingDirNotDirectory)
	})
}
