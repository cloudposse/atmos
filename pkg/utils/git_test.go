package utils

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				err := os.Chdir(tmpDir)
				require.NoError(t, err)
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
				err := os.Chdir(tmpDir)
				require.NoError(t, err)
				return ""
			},
			expected:    "/fallback/value",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original working directory
			originalDir, err := os.Getwd()
			require.NoError(t, err)
			defer os.Chdir(originalDir)

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
	// This test verifies that TEST_GIT_ROOT properly overrides Git detection

	// Save original directory
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	t.Run("TEST_GIT_ROOT overrides Git detection", func(t *testing.T) {
		// Test 1: TEST_GIT_ROOT overrides any Git detection
		mockRoot := "/mock/override/path"
		t.Setenv("TEST_GIT_ROOT", mockRoot)

		result, err := ProcessTagGitRoot("!repo-root")
		assert.NoError(t, err)
		assert.Equal(t, mockRoot, result)

		// Test 2: TEST_GIT_ROOT works with a path suffix
		result, err = ProcessTagGitRoot("!repo-root /some/path")
		assert.NoError(t, err)
		assert.Equal(t, mockRoot, result, "TEST_GIT_ROOT should override even when input has a suffix")
	})

	t.Run("Empty TEST_GIT_ROOT falls back to default value", func(t *testing.T) {
		// Don't set TEST_GIT_ROOT - it will be unset

		// Create a temp directory without a Git repo
		tmpDir := t.TempDir()
		err := os.Chdir(tmpDir)
		require.NoError(t, err)
		defer os.Chdir(originalDir)

		result, err := ProcessTagGitRoot("!repo-root /default/path")
		assert.NoError(t, err)
		assert.Equal(t, "/default/path", result, "Should return default value when not in Git repo and no TEST_GIT_ROOT")
	})
}
