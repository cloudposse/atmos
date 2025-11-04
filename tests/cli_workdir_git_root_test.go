package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testhelpers "github.com/cloudposse/atmos/tests/testhelpers"
)

// TestWorkdirGitRootDetection tests that atmos can find atmos.yaml at git root
// when running from a component subdirectory within a git repository.
//
// This reproduces the user-reported issue where:
// - User is inside components/terraform/mycomponent/ directory.
// - atmos.yaml exists at the git root (3 levels up).
// - Running `atmos terraform plan` fails with "atmos.yaml CLI config file was not found".
// - This should work automatically via git root detection, but currently doesn't.
func TestWorkdirGitRootDetection(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Get absolute path to fixtures with atmos.yaml at root.
	originalWd, err := os.Getwd()
	require.NoError(t, err)

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "basic")
	absFixturesDir, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)
	require.DirExists(t, absFixturesDir, "fixtures directory should exist")

	// Verify atmos.yaml exists at fixtures root.
	atmosYamlPath := filepath.Join(absFixturesDir, "atmos.yaml")
	require.FileExists(t, atmosYamlPath, "atmos.yaml should exist at fixtures root")

	// Set TEST_GIT_ROOT to simulate git root detection.
	t.Setenv("TEST_GIT_ROOT", absFixturesDir)

	// Path to a terraform component subdirectory (simulating user's scenario).
	componentDir := filepath.Join(absFixturesDir, "components", "terraform", "mock")
	require.DirExists(t, componentDir, "component directory should exist")

	tests := []struct {
		name          string
		startDir      string
		command       []string
		expectError   bool
		checkOutput   func(t *testing.T, stdout, stderr string)
		skipReason    string
		expectedError string // Expected error message when test is expected to fail
	}{
		{
			name:     "describe stacks from component directory (control - should work)",
			startDir: componentDir,
			command:  []string{"describe", "stacks"},
			// This currently FAILS but SHOULD work - atmos should find atmos.yaml at git root.
			expectError:   true,
			expectedError: "atmos.yaml CLI config file was not found",
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// KNOWN BUG: This assertion documents the current broken behavior.
				// When git root detection is implemented, this test should be updated to:
				// assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
				assert.Contains(t, stderr, "atmos.yaml CLI config file was not found",
					"BUG: atmos.yaml not found from component dir - git root detection not working")
			},
		},
		{
			name:     "terraform plan from component directory (user's exact scenario)",
			startDir: componentDir,
			command:  []string{"terraform", "plan", "mycomponent", "--stack", "nonprod"},
			// This currently FAILS but SHOULD work - atmos should find atmos.yaml at git root.
			expectError:   true,
			expectedError: "atmos.yaml CLI config file was not found",
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// KNOWN BUG: This assertion documents the current broken behavior.
				// When git root detection is implemented, this test should be updated to:
				// assert.NotContains(t, stderr, "atmos.yaml CLI config file was not found")
				assert.Contains(t, stderr, "atmos.yaml CLI config file was not found",
					"BUG: atmos.yaml not found from component dir - git root detection not working")
			},
		},
		{
			name:     "terraform plan from nested component subdirectory",
			startDir: componentDir,
			command:  []string{"terraform", "plan", "mycomponent", "--stack", "nonprod"},
			// This currently FAILS but SHOULD work.
			expectError:   true,
			expectedError: "atmos.yaml CLI config file was not found",
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.Contains(t, stderr, "atmos.yaml  CLI config file was not found",
					"BUG: atmos.yaml not found from nested dir - git root detection not working")
			},
		},
		{
			name:     "list stacks from stacks directory",
			startDir: filepath.Join(fixturesDir, "stacks"),
			command:  []string{"list", "stacks"},
			// This currently FAILS but SHOULD work.
			expectError:   true,
			expectedError: "atmos.yaml CLI config file was not found",
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.Contains(t, stderr, "atmos.yaml  CLI config file was not found",
					"BUG: atmos.yaml not found from stacks dir - git root detection not working")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
				return
			}

			// Use t.Chdir() to change to the starting directory (automatic cleanup).
			t.Chdir(tt.startDir)

			// Log current directory for debugging.
			currentDir, _ := os.Getwd()
			t.Logf("Running from directory: %s", currentDir)
			t.Logf("Git root (simulated via TEST_GIT_ROOT): %s", absFixturesDir)

			// Run the command.
			cmd := atmosRunner.Command(tt.command...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()

			//nolint:nestif // Test code complexity is acceptable for clarity
			if tt.expectError {
				// Command is expected to fail.
				if err == nil {
					t.Logf("Command succeeded unexpectedly")
					t.Logf("Stdout: %s", stdout.String())
					t.Logf("Stderr: %s", stderr.String())
				} else {
					t.Logf("Command failed as expected with: %v", err)
					t.Logf("Stderr: %s", stderr.String())
				}
			} else {
				if err != nil {
					t.Logf("Command failed with error: %v", err)
					t.Logf("Stdout: %s", stdout.String())
					t.Logf("Stderr: %s", stderr.String())
				}
				assert.NoError(t, err, "Command should succeed once git root detection is implemented")
			}

			// Run output checks if provided.
			if tt.checkOutput != nil {
				tt.checkOutput(t, stdout.String(), stderr.String())
			}
		})
	}
}

// TestWorkdirGitRootDetectionWithChdir tests that --chdir flag works as a workaround
// for the missing git root detection.
func TestWorkdirGitRootDetectionWithChdir(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Get absolute path to fixtures with atmos.yaml at root.
	originalWd, err := os.Getwd()
	require.NoError(t, err)

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "basic")
	absFixturesDir, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)
	require.DirExists(t, absFixturesDir, "fixtures directory should exist")

	// Verify atmos.yaml exists at fixtures root.
	atmosYamlPath := filepath.Join(absFixturesDir, "atmos.yaml")
	require.FileExists(t, atmosYamlPath, "atmos.yaml should exist at fixtures root")

	// Path to a terraform component subdirectory.
	componentDir := filepath.Join(absFixturesDir, "components", "terraform", "mock")
	require.DirExists(t, componentDir, "component directory should exist")

	t.Run("terraform plan with --chdir workaround from component directory", func(t *testing.T) {
		// Use t.Chdir() to change to component directory (automatic cleanup).
		t.Chdir(componentDir)

		// Run with --chdir pointing to git root (the workaround).
		cmd := atmosRunner.Command("--chdir", absFixturesDir, "terraform", "plan", "mycomponent", "--stack", "nonprod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Command failed with error: %v", err)
			t.Logf("Stderr: %s", stderr.String())
		}

		// With --chdir flag (after PR #1751), atmos.yaml should be found.
		// Command may still fail for other reasons (terraform not initialized, etc.)
		// but NOT because atmos.yaml is missing.
		assert.NotContains(t, stderr.String(), "atmos.yaml CLI config file was not found",
			"With --chdir flag, atmos.yaml should be found at specified root")
	})
}
