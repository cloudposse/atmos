package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessTagCwd(t *testing.T) {
	// Save and restore original working directory.
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		err := os.Chdir(originalDir)
		if err != nil {
			t.Errorf("failed to restore original directory: %v", err)
		}
	})

	tests := []struct {
		name     string
		input    string
		expected func(cwd string) string
	}{
		{
			name:  "Basic !cwd tag returns current working directory",
			input: "!cwd",
			expected: func(cwd string) string {
				return cwd
			},
		},
		{
			name:  "!cwd with trailing space",
			input: "!cwd ",
			expected: func(cwd string) string {
				return cwd
			},
		},
		{
			name:  "!cwd with relative path",
			input: "!cwd ./relative/path",
			expected: func(cwd string) string {
				return filepath.Join(cwd, ".", "relative", "path")
			},
		},
		{
			name:  "!cwd with path without dot prefix",
			input: "!cwd subdir",
			expected: func(cwd string) string {
				return filepath.Join(cwd, "subdir")
			},
		},
		{
			name:  "!cwd with parent directory path",
			input: "!cwd ../parent",
			expected: func(cwd string) string {
				return filepath.Join(cwd, "..", "parent")
			},
		},
		{
			name:  "!cwd with multiple spaces before path",
			input: "!cwd   multiple/spaces",
			expected: func(cwd string) string {
				return filepath.Join(cwd, "multiple", "spaces")
			},
		},
		{
			name:  "!cwd with complex relative path",
			input: "!cwd ./components/terraform/vpc",
			expected: func(cwd string) string {
				return filepath.Join(cwd, ".", "components", "terraform", "vpc")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get current working directory.
			cwd, err := os.Getwd()
			require.NoError(t, err)

			result, err := ProcessTagCwd(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected(cwd), result)
		})
	}
}

func TestProcessTagCwd_DifferentWorkingDirectory(t *testing.T) {
	// Create a temp directory and change to it.
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	t.Run("Returns temp directory as CWD", func(t *testing.T) {
		result, err := ProcessTagCwd("!cwd")
		assert.NoError(t, err)
		assert.Equal(t, tmpDir, result)
	})

	t.Run("Joins temp directory with relative path", func(t *testing.T) {
		result, err := ProcessTagCwd("!cwd ./mypath")
		assert.NoError(t, err)
		assert.Equal(t, filepath.Join(tmpDir, ".", "mypath"), result)
	})
}

func TestProcessTagGitRoot(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		testGitRoot string
		setupFunc   func(t *testing.T) string
		expected    string
		expectError bool
	}{
		{
			name:        "TEST_GIT_ROOT environment variable overrides Git detection",
			input:       "!repo-root",
			testGitRoot: "/mock/git/root",
			expected:    "/mock/git/root",
			expectError: false,
		},
		{
			name:        "TEST_GIT_ROOT with default value",
			input:       "!repo-root /default/path",
			testGitRoot: "/override/root",
			expected:    "/override/root",
			expectError: false,
		},
		{
			name:        "TEST_GIT_ROOT empty with default value",
			input:       "!repo-root /default/fallback",
			testGitRoot: "",
			setupFunc: func(t *testing.T) string {
				// Change to a directory without a Git repo
				// The function should return the default value
				tmpDir := t.TempDir()
				t.Chdir(tmpDir)
				return "/default/fallback"
			},
			expected:    "/default/fallback",
			expectError: false,
		},
		{
			name:        "Returns default value when not in Git repo and no TEST_GIT_ROOT",
			input:       "!repo-root /fallback/value",
			testGitRoot: "",
			setupFunc: func(t *testing.T) string {
				// Change to a directory without .git
				tmpDir := t.TempDir()
				t.Chdir(tmpDir)
				return ""
			},
			expected:    "/fallback/value",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set TEST_GIT_ROOT if provided
			if tt.testGitRoot != "" {
				t.Setenv("TEST_GIT_ROOT", tt.testGitRoot)
			}

			// Run setup function if provided
			var expectedPath string
			if tt.setupFunc != nil {
				expectedPath = tt.setupFunc(t)
			}

			// Call the function
			result, err := ProcessTagGitRoot(tt.input)

			// Check error expectation
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Check result
			if tt.expected != "" {
				assert.Equal(t, tt.expected, result)
			} else if expectedPath != "" {
				assert.Equal(t, expectedPath, result)
			}
		})
	}
}

func TestProcessTagGitRoot_Integration(t *testing.T) {
	// This test verifies that TEST_GIT_ROOT properly overrides Git detection.

	t.Run("TEST_GIT_ROOT overrides Git detection", func(t *testing.T) {
		// Test 1: TEST_GIT_ROOT overrides any Git detection.
		mockRoot := "/mock/override/path"
		t.Setenv("TEST_GIT_ROOT", mockRoot)

		result, err := ProcessTagGitRoot("!repo-root")
		assert.NoError(t, err)
		assert.Equal(t, mockRoot, result)

		// Test 2: TEST_GIT_ROOT works with a path suffix.
		result, err = ProcessTagGitRoot("!repo-root /some/path")
		assert.NoError(t, err)
		assert.Equal(t, mockRoot, result, "TEST_GIT_ROOT should override even when input has a suffix")
	})

	t.Run("Empty TEST_GIT_ROOT falls back to default value", func(t *testing.T) {
		// Don't set TEST_GIT_ROOT - it will be unset.

		// Create a temp directory without a Git repo.
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		result, err := ProcessTagGitRoot("!repo-root /default/path")
		assert.NoError(t, err)
		assert.Equal(t, "/default/path", result, "Should return default value when not in Git repo and no TEST_GIT_ROOT")
	})
}

func TestProcessTagGitRoot_RealGitRepo(t *testing.T) {
	// This test runs in the actual atmos repo to test real Git detection.
	// Skip if TEST_GIT_ROOT is already set (test isolation mode).

	t.Run("Detects real Git root without TEST_GIT_ROOT override", func(t *testing.T) {
		// Save and check if we're in a git repo.
		originalDir, err := os.Getwd()
		require.NoError(t, err)
		t.Cleanup(func() {
			err := os.Chdir(originalDir)
			if err != nil {
				t.Errorf("failed to restore original directory: %v", err)
			}
		})

		// The atmos repo root should be detected.
		result, err := ProcessTagGitRoot("!repo-root")
		require.NoError(t, err)

		// The result should be an absolute path containing "atmos".
		assert.True(t, filepath.IsAbs(result), "Git root should be an absolute path")
		assert.Contains(t, result, "atmos", "Should be in the atmos repository")

		// Verify .git exists at the detected root.
		gitDir := filepath.Join(result, ".git")
		_, statErr := os.Stat(gitDir)
		assert.NoError(t, statErr, ".git directory should exist at detected root")
	})

	t.Run("Returns error without default value when not in Git repo", func(t *testing.T) {
		// Create a temp directory that is not a Git repo.
		tmpDir := t.TempDir()
		t.Chdir(tmpDir)

		// Without a default value, this should return an error.
		_, err := ProcessTagGitRoot("!repo-root")
		assert.Error(t, err, "Should return error when not in Git repo and no default value")
		assert.Contains(t, err.Error(), "failed to open Git repository")
	})
}
