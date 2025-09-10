package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectProjectContext(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string // Setup function that returns the test directory
		expected string
	}{
		{
			name: "tools subdirectory - gotcha",
			setup: func(t *testing.T) string {
				// Create a temp directory structure simulating /path/to/project/tools/gotcha
				tempDir := t.TempDir()
				toolDir := filepath.Join(tempDir, "project", "tools", "gotcha")
				require.NoError(t, os.MkdirAll(toolDir, 0o755))

				// Create a .git directory at the project root
				gitDir := filepath.Join(tempDir, "project", ".git")
				require.NoError(t, os.MkdirAll(gitDir, 0o755))

				return toolDir
			},
			expected: "gotcha",
		},
		{
			name: "tools subdirectory - other tool",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				toolDir := filepath.Join(tempDir, "myrepo", "tools", "mytool", "subdir")
				require.NoError(t, os.MkdirAll(toolDir, 0o755))

				// Create a .git directory at the repo root
				gitDir := filepath.Join(tempDir, "myrepo", ".git")
				require.NoError(t, os.MkdirAll(gitDir, 0o755))

				return toolDir
			},
			expected: "mytool",
		},
		{
			name: "project root directory",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				projectDir := filepath.Join(tempDir, "atmos")
				require.NoError(t, os.MkdirAll(projectDir, 0o755))

				// Create a .git directory
				gitDir := filepath.Join(projectDir, ".git")
				require.NoError(t, os.MkdirAll(gitDir, 0o755))

				return projectDir
			},
			expected: "atmos",
		},
		{
			name: "project subdirectory (not tools)",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				subDir := filepath.Join(tempDir, "myproject", "pkg", "config")
				require.NoError(t, os.MkdirAll(subDir, 0o755))

				// Create a .git directory at the project root
				gitDir := filepath.Join(tempDir, "myproject", ".git")
				require.NoError(t, os.MkdirAll(gitDir, 0o755))

				return subDir
			},
			expected: "myproject",
		},
		{
			name: "no git directory - fallback to current dir name",
			setup: func(t *testing.T) string {
				tempDir := t.TempDir()
				workDir := filepath.Join(tempDir, "workspace")
				require.NoError(t, os.MkdirAll(workDir, 0o755))
				return workDir
			},
			expected: "workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save current directory and restore after test
			originalDir, err := os.Getwd()
			require.NoError(t, err)
			defer func() {
				require.NoError(t, os.Chdir(originalDir))
			}()

			// Setup test directory and change to it
			testDir := tt.setup(t)
			require.NoError(t, os.Chdir(testDir))

			// Test the function
			result := detectProjectContext()
			assert.Equal(t, tt.expected, result, "Project context mismatch for directory: %s", testDir)
		})
	}
}
