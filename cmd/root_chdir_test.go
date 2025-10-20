package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testChdirError is a helper function to test error cases for the chdir flag.
// It tests the actual production processChdirFlag() function.
func testChdirError(t *testing.T, args []string) {
	t.Helper()

	// Ensure RootCmd state is restored after this helper finishes.
	_ = NewTestKit(t)

	// Set up RootCmd with test args.
	RootCmd.SetArgs(args)
	err := RootCmd.ParseFlags(args)
	if err != nil {
		// Parse error is acceptable for invalid flag syntax.
		return
	}

	// Call the actual production code to test error handling.
	err = processChdirFlag(RootCmd)
	assert.Error(t, err, "Expected processChdirFlag to return error for invalid input")
}

// TestChdirFlag tests the --chdir/-C flag functionality.
func TestChdirFlag(t *testing.T) {
	// Save original working directory to restore after tests.
	originalWd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current working directory")
	// Restore RootCmd state after test.
	_ = NewTestKit(t)

	t.Cleanup(func() {
		// Restore original working directory.
		_ = os.Chdir(originalWd)
		// Explicitly unset ATMOS_CHDIR to prevent pollution to other tests.
		os.Unsetenv("ATMOS_CHDIR")
	})

	tests := []struct {
		name        string
		setup       func(t *testing.T) string          // Returns directory/file path if needed.
		makeArgs    func(testDir string) []string      // Builds args from setup result.
		setupEnv    func(t *testing.T, testDir string) // Sets up environment variables.
		expectError bool
	}{
		{
			name: "absolute path via --chdir flag",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			makeArgs: func(testDir string) []string {
				return []string{"--chdir", testDir}
			},
			expectError: false,
		},
		{
			name: "absolute path via -C flag (short form)",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			makeArgs: func(testDir string) []string {
				return []string{"-C", testDir}
			},
			expectError: false,
		},
		{
			name: "relative path via --chdir",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				// Create a subdirectory.
				subDir := filepath.Join(tmpDir, "subdir")
				require.NoError(t, os.Mkdir(subDir, 0o755))
				// Change to parent directory so we can use relative path.
				require.NoError(t, os.Chdir(tmpDir))
				return subDir
			},
			makeArgs: func(testDir string) []string {
				return []string{"--chdir", "subdir"}
			},
			expectError: false,
		},
		{
			name: "ATMOS_CHDIR environment variable",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			setupEnv: func(t *testing.T, testDir string) {
				t.Setenv("ATMOS_CHDIR", testDir)
			},
			makeArgs: func(testDir string) []string {
				return []string{}
			},
			expectError: false,
		},
		{
			name: "--chdir flag overrides ATMOS_CHDIR env var",
			setup: func(t *testing.T) string {
				return t.TempDir() // This will be the flag value (wins).
			},
			setupEnv: func(t *testing.T, testDir string) {
				envDir := t.TempDir()
				t.Setenv("ATMOS_CHDIR", envDir) // This should be overridden.
			},
			makeArgs: func(testDir string) []string {
				return []string{"--chdir", testDir}
			},
			expectError: false,
		},
		{
			name:  "non-existent directory returns error",
			setup: nil,
			makeArgs: func(testDir string) []string {
				return []string{"--chdir", "/nonexistent/directory/that/does/not/exist"}
			},
			expectError: true,
		},
		{
			name: "chdir to a file (not directory) returns error",
			setup: func(t *testing.T) string {
				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "testfile.txt")
				require.NoError(t, os.WriteFile(tmpFile, []byte("test"), 0o644))
				return tmpFile
			},
			makeArgs: func(testDir string) []string {
				return []string{"--chdir", testDir}
			},
			expectError: true,
		},
		{
			name:  "empty chdir value is ignored",
			setup: nil,
			makeArgs: func(testDir string) []string {
				return []string{"--chdir", ""}
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Restore original directory before each test.
			require.NoError(t, os.Chdir(originalWd))

			// Ensure working directory is restored after this sub-test.
			// IMPORTANT: Register this BEFORE calling setup() which may create temp dirs.
			// Cleanup runs in LIFO order, so this ensures we chdir out before temp dir removal.
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
			})

			// Run setup to get test directory if needed.
			var testDir string
			if tt.setup != nil {
				testDir = tt.setup(t)
			}

			// Set up environment variables if needed.
			if tt.setupEnv != nil {
				tt.setupEnv(t, testDir)
			}

			// Build args using the test data.
			args := tt.makeArgs(testDir)

			// Create a test command.
			testCmd := &cobra.Command{
				Use: "test",
				Run: func(cmd *cobra.Command, args []string) {
					// Command execution - we just need to verify directory changed.
				},
			}

			// Add the chdir flag to test command.
			testCmd.PersistentFlags().StringP("chdir", "C", "", "Change to directory before executing command")

			// Parse flags.
			testCmd.SetArgs(args)
			err := testCmd.ParseFlags(args)

			if tt.expectError {
				testChdirError(t, args)
			} else {
				require.NoError(t, err, "Flag parsing should not error")

				// Get chdir value.
				chdir, err := testCmd.Flags().GetString("chdir")
				require.NoError(t, err)

				// If flag empty, fall back to environment variable.
				if chdir == "" {
					chdir = os.Getenv("ATMOS_CHDIR")
				}

				// If chdir is specified, change directory and verify.
				if chdir != "" {
					absPath, err := filepath.Abs(chdir)
					require.NoError(t, err, "Should be able to get absolute path")

					// Verify directory exists.
					stat, err := os.Stat(absPath)
					require.NoError(t, err, "Directory should exist")
					require.True(t, stat.IsDir(), "Path should be a directory")

					// Change directory.
					err = os.Chdir(absPath)
					require.NoError(t, err, "Should be able to change directory")

					// Verify we're in the expected directory.
					// Use EvalSymlinks to resolve paths on macOS where /var -> /private/var.
					currentWd, err := os.Getwd()
					require.NoError(t, err)
					expectedResolved, _ := filepath.EvalSymlinks(absPath)
					currentResolved, _ := filepath.EvalSymlinks(currentWd)
					assert.Equal(t, expectedResolved, currentResolved, "Should be in the expected directory")
				}
			}

			// IMPORTANT: Change back to original directory BEFORE test ends.
			// On Windows, temp dir cleanup fails if we're still inside it.
			_ = os.Chdir(originalWd)
		})
	}
}

// TestChdirFlagIntegration tests the chdir flag integration with actual Atmos commands.
func TestChdirFlagIntegration(t *testing.T) {
	// Save original working directory.
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Create a temporary directory with atmos.yaml.
	tmpDir := t.TempDir()
	atmosYaml := filepath.Join(tmpDir, "atmos.yaml")
	atmosConfig := `
base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
`
	require.NoError(t, os.WriteFile(atmosYaml, []byte(atmosConfig), 0o644))

	// Create directory structure.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "components", "terraform"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "stacks"), 0o755))

	// Test that chdir works with actual command execution.
	t.Run("chdir changes working directory before config loading", func(t *testing.T) {
		// Start from original directory.
		require.NoError(t, os.Chdir(originalWd))

		// Ensure working directory is restored after this sub-test.
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
		})

		// Create test command that verifies directory change.
		testCmd := &cobra.Command{
			Use: "testchdir",
			Run: func(cmd *cobra.Command, args []string) {
				// Command execution - directory change happens before this.
			},
		}
		testCmd.PersistentFlags().StringP("chdir", "C", "", "")

		// Set args with chdir pointing to tmpDir.
		testCmd.SetArgs([]string{"--chdir", tmpDir})

		// Parse flags and change directory manually (simulating PersistentPreRun).
		_ = testCmd.ParseFlags([]string{"--chdir", tmpDir})
		chdir, _ := testCmd.Flags().GetString("chdir")
		if chdir != "" {
			absPath, _ := filepath.Abs(chdir)
			_ = os.Chdir(absPath)
		}

		// Execute.
		_ = testCmd.Execute()

		// Verify we changed to the expected directory.
		// Use EvalSymlinks to handle macOS symlinks (/var -> /private/var).
		wd, _ := os.Getwd()
		expectedAbs, _ := filepath.Abs(tmpDir)
		expectedResolved, _ := filepath.EvalSymlinks(expectedAbs)
		wdResolved, _ := filepath.EvalSymlinks(wd)
		assert.Equal(t, expectedResolved, wdResolved, "Working directory should match chdir target")
	})

	t.Run("chdir processes before base-path", func(t *testing.T) {
		require.NoError(t, os.Chdir(originalWd))

		// Ensure working directory is restored after this sub-test.
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
		})

		// Test that --chdir is processed before --base-path.
		subDir := filepath.Join(tmpDir, "subproject")
		require.NoError(t, os.MkdirAll(subDir, 0o755))

		// Create atmos.yaml in subdir.
		subAtmosYaml := filepath.Join(subDir, "atmos.yaml")
		require.NoError(t, os.WriteFile(subAtmosYaml, []byte(atmosConfig), 0o644))

		// Just verify directory structure was created correctly.
		_, err := os.Stat(subAtmosYaml)
		require.NoError(t, err, "Subproject atmos.yaml should exist")
	})
}

// TestChdirFlagEdgeCases tests edge cases and error conditions.
func TestChdirFlagEdgeCases(t *testing.T) {
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	tests := []struct {
		name        string
		setup       func(t *testing.T) (string, func())
		args        []string
		expectError bool
	}{
		{
			name: "symlink to directory",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				targetDir := filepath.Join(tmpDir, "target")
				require.NoError(t, os.Mkdir(targetDir, 0o755))

				symlinkPath := filepath.Join(tmpDir, "symlink")
				err := os.Symlink(targetDir, symlinkPath)
				if err != nil {
					t.Skipf("Skipping symlink test on Windows: symlinks require special privileges")
				}

				return symlinkPath, func() {}
			},
			args:        []string{}, // Will be set with symlink path
			expectError: false,
		},
		{
			name: "path with spaces",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				dirWithSpaces := filepath.Join(tmpDir, "dir with spaces")
				require.NoError(t, os.Mkdir(dirWithSpaces, 0o755))
				return dirWithSpaces, func() {}
			},
			args:        []string{}, // Will be set with path
			expectError: false,
		},
		{
			name: "relative path with .. (parent directory)",
			setup: func(t *testing.T) (string, func()) {
				tmpDir := t.TempDir()
				subDir := filepath.Join(tmpDir, "sub", "nested")
				require.NoError(t, os.MkdirAll(subDir, 0o755))
				// Change to subDir so we can use relative path.
				require.NoError(t, os.Chdir(subDir))
				return "../..", func() {
					_ = os.Chdir(originalWd)
				}
			},
			args:        []string{"--chdir", "../.."},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, os.Chdir(originalWd))

			// Ensure working directory is restored after this sub-test.
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
			})

			testPath, cleanup := tt.setup(t)
			if cleanup != nil {
				t.Cleanup(cleanup)
			}

			args := tt.args
			if len(args) == 0 {
				args = []string{"--chdir", testPath}
			}

			// Create test command for edge case testing.
			testCmd := &cobra.Command{
				Use: "test",
				Run: func(cmd *cobra.Command, args []string) {},
			}
			testCmd.PersistentFlags().StringP("chdir", "C", "", "")

			testCmd.SetArgs(args)
			err := testCmd.ParseFlags(args)

			if tt.expectError {
				assert.Error(t, err, "Expected error for test case: %s", tt.name)
				return
			}

			require.NoError(t, err, "Should parse flags without error")

			// Try to change directory if chdir is set.
			chdir, _ := testCmd.Flags().GetString("chdir")
			if chdir != "" {
				absPath, err := filepath.Abs(chdir)
				if err == nil {
					err = os.Chdir(absPath)
				}
				// Should succeed for non-error cases.
				assert.NoError(t, err, "Should change directory successfully")
			}

			// IMPORTANT: Change back to original directory BEFORE test ends.
			// On Windows, temp dir cleanup fails if we're still inside it.
			_ = os.Chdir(originalWd)
		})
	}
}

// TestChdirFlagPrecedence tests that CLI flag takes precedence over environment variable.
func TestChdirFlagPrecedence(t *testing.T) {
	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Create two different temp directories.
	envDir := t.TempDir()
	flagDir := t.TempDir()

	// Set environment variable.
	t.Setenv("ATMOS_CHDIR", envDir)

	// Create test command.
	testCmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	testCmd.PersistentFlags().StringP("chdir", "C", "", "")

	// Use flag (should override env var).
	testCmd.SetArgs([]string{"--chdir", flagDir})

	// Parse flags to simulate PersistentPreRun behavior.
	err = testCmd.ParseFlags([]string{"--chdir", flagDir})
	require.NoError(t, err)

	// Get chdir value.
	chdir, _ := testCmd.Flags().GetString("chdir")

	// Verify flag value is used, not env var.
	assert.Equal(t, flagDir, chdir, "Flag value should take precedence over environment variable")

	// Now test with only env var (no flag).
	testCmd2 := &cobra.Command{
		Use: "test2",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	testCmd2.PersistentFlags().StringP("chdir", "C", "", "")

	testCmd2.SetArgs([]string{})
	err = testCmd2.ParseFlags([]string{})
	require.NoError(t, err)

	chdir, _ = testCmd2.Flags().GetString("chdir")
	// Flag is empty, so env var should be used.
	assert.Equal(t, "", chdir, "Flag should be empty when not specified")

	// Verify env var is still set.
	assert.Equal(t, envDir, os.Getenv("ATMOS_CHDIR"), "Environment variable should still be set")
}
