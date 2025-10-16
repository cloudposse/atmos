package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessChdirFlag tests the processChdirFlag function directly.
func TestProcessChdirFlag(t *testing.T) {
	// Save original working directory.
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	tests := []struct {
		name        string
		flagValue   string
		envValue    string
		setup       func(t *testing.T) string // Returns expected directory.
		expectError bool
		errorMsg    string
	}{
		{
			name:      "no chdir flag or env var",
			flagValue: "",
			envValue:  "",
			setup: func(t *testing.T) string {
				return originalWd // Should stay in original directory.
			},
			expectError: false,
		},
		{
			name: "valid absolute path via flag",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			expectError: false,
		},
		{
			name: "valid absolute path via env var",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return tmpDir
			},
			expectError: false,
		},
		{
			name: "flag takes precedence over env var",
			setup: func(t *testing.T) string {
				tmpDir1 := t.TempDir()
				tmpDir2 := t.TempDir()
				t.Setenv("ATMOS_CHDIR", tmpDir2) // Set env var.
				return tmpDir1                   // Flag should win.
			},
			expectError: false,
		},
		{
			name:      "non-existent directory",
			flagValue: "/this/path/does/not/exist",
			setup: func(t *testing.T) string {
				return ""
			},
			expectError: true,
			errorMsg:    "directory does not exist",
		},
		{
			name: "path is a file, not directory",
			setup: func(t *testing.T) string {
				tmpFile := filepath.Join(t.TempDir(), "file.txt")
				require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o644))
				return tmpFile
			},
			expectError: true,
			errorMsg:    "not a directory",
		},
		{
			name: "relative path",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "subdir")
				require.NoError(t, os.Mkdir(subDir, 0o755))
				// Change to parent so we can use relative path.
				require.NoError(t, os.Chdir(tmpDir))
				return subDir
			},
			flagValue:   "subdir",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Restore original directory before each test.
			require.NoError(t, os.Chdir(originalWd))

			// Cleanup after test.
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
			})

			var expectedDir string
			if tt.setup != nil {
				expectedDir = tt.setup(t)
			}

			// Create test command with chdir flag.
			testCmd := &cobra.Command{
				Use: "test",
				Run: func(cmd *cobra.Command, args []string) {},
			}
			testCmd.PersistentFlags().StringP("chdir", "C", "", "")

			// Set flag value if provided, otherwise use setup result.
			flagValue := tt.flagValue
			if flagValue == "" && expectedDir != "" && tt.name != "no chdir flag or env var" {
				flagValue = expectedDir
			}

			if flagValue != "" {
				testCmd.SetArgs([]string{"--chdir", flagValue})
				require.NoError(t, testCmd.ParseFlags([]string{"--chdir", flagValue}))
			}

			// Set env var if provided in test case.
			if tt.envValue != "" {
				t.Setenv("ATMOS_CHDIR", tt.envValue)
			}

			// Call the function.
			err := processChdirFlag(testCmd)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)

				// Verify working directory changed.
				if expectedDir != "" && expectedDir != originalWd {
					currentWd, _ := os.Getwd()
					// Use EvalSymlinks for macOS compatibility.
					expectedResolved, _ := filepath.EvalSymlinks(expectedDir)
					currentResolved, _ := filepath.EvalSymlinks(currentWd)
					assert.Equal(t, expectedResolved, currentResolved)
				}
			}
		})
	}
}

// TestProcessChdirFlagWithEnvVar tests environment variable handling.
func TestProcessChdirFlagWithEnvVar(t *testing.T) {
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	tmpDir := t.TempDir()
	t.Setenv("ATMOS_CHDIR", tmpDir)

	testCmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	testCmd.PersistentFlags().StringP("chdir", "C", "", "")

	err = processChdirFlag(testCmd)
	require.NoError(t, err)

	currentWd, _ := os.Getwd()
	expectedResolved, _ := filepath.EvalSymlinks(tmpDir)
	currentResolved, _ := filepath.EvalSymlinks(currentWd)
	assert.Equal(t, expectedResolved, currentResolved)
}

// TestProcessChdirFlagPrecedence tests that flag takes precedence over env var.
func TestProcessChdirFlagPrecedence(t *testing.T) {
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	envDir := t.TempDir()
	flagDir := t.TempDir()

	t.Setenv("ATMOS_CHDIR", envDir)

	testCmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	testCmd.PersistentFlags().StringP("chdir", "C", "", "")
	testCmd.SetArgs([]string{"--chdir", flagDir})
	require.NoError(t, testCmd.ParseFlags([]string{"--chdir", flagDir}))

	err = processChdirFlag(testCmd)
	require.NoError(t, err)

	// Should be in flagDir, not envDir.
	currentWd, _ := os.Getwd()
	expectedResolved, _ := filepath.EvalSymlinks(flagDir)
	currentResolved, _ := filepath.EvalSymlinks(currentWd)
	assert.Equal(t, expectedResolved, currentResolved)
}
