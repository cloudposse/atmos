package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestChdirWithTerraformCommands tests the --chdir flag with various terraform commands.
// This reproduces the issue reported where --chdir works with some commands (like generate varfile)
// but fails with others (like terraform plan).
func TestChdirWithTerraformCommands(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for chdir terraform commands test", "coverageEnabled", coverDir != "")
	}

	RequireTerraform(t)

	// Get absolute path to the fixtures directory.
	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "basic")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err, "Failed to get absolute path to fixtures")

	// Verify fixtures directory exists.
	_, err = os.Stat(absFixturesPath)
	require.NoError(t, err, "Fixtures directory should exist at %s", absFixturesPath)

	// Create a subdirectory to simulate running from a component directory (like in Atlantis).
	componentDir := filepath.Join(absFixturesPath, "components", "terraform", "mock")
	require.DirExists(t, componentDir, "Component directory should exist")

	tests := []struct {
		name        string
		args        []string
		expectError bool
		skipReason  string
		checkOutput func(t *testing.T, stdout, stderr string)
	}{
		{
			name:        "terraform generate varfile with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// Should succeed and generate varfile.
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir")
			},
		},
		{
			name:        "terraform plan with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "plan", "mycomponent", "--stack", "nonprod"},
			expectError: true, // May fail due to missing terraform initialization, but NOT due to missing atmos.yaml.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// The key assertion: atmos.yaml should be found with --chdir.
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform plan")
			},
		},
		{
			name:        "terraform validate with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "validate", "mycomponent", "--stack", "nonprod"},
			expectError: true, // May fail due to terraform not being initialized, but NOT due to missing atmos.yaml.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform validate")
			},
		},
		{
			name: "terraform init with absolute --chdir path",
			args: []string{"--chdir", absFixturesPath, "terraform", "init", "mycomponent", "--stack", "nonprod"},
			// Skip because init might require actual terraform state/backend config.
			skipReason: "Skipping terraform init - requires backend configuration",
		},
		{
			name:        "terraform workspace with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "workspace", "mycomponent", "--stack", "nonprod"},
			expectError: true, // May fail due to terraform not being initialized, but NOT due to missing atmos.yaml.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform workspace")
			},
		},
		{
			name:        "terraform generate backend with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "generate", "backend", "mycomponent", "--stack", "nonprod"},
			expectError: true, // May fail due to missing backend_type configuration, but NOT due to missing atmos.yaml.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform generate backend")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}

			// Change to component directory to simulate Atlantis behavior.
			// This is the scenario described in the bug report:
			// "The reason I'm trying to change dir is because Atlantis starts its process in the root directory of terraform"
			t.Chdir(componentDir)
			require.NoError(t, err, "Should be able to change to component directory")

			// Restore directory after test.
			t.Cleanup(func() {
				t.Chdir(startingDir)
			})

			// Run the command with --chdir pointing back to the repo root.
			cmd := atmosRunner.Command(tt.args...)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err = cmd.Run()

			if tt.expectError {
				assert.Error(t, err, "Expected command to fail")
			} else {
				if err != nil {
					t.Logf("Command failed with error: %v", err)
					t.Logf("Stdout: %s", stdout.String())
					t.Logf("Stderr: %s", stderr.String())
				}
				assert.NoError(t, err, "Command should succeed with --chdir")
			}

			// Run output checks if provided.
			if tt.checkOutput != nil {
				tt.checkOutput(t, stdout.String(), stderr.String())
			}
		})
	}
}

// TestChdirWithRelativePaths tests --chdir with relative paths for terraform commands.
func TestChdirWithRelativePaths(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	RequireTerraform(t)

	startingDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		t.Chdir(startingDir)
	})

	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "basic")
	componentDir := filepath.Join(fixturesDir, "components", "terraform", "mock")
	require.DirExists(t, componentDir)

	t.Run("terraform generate varfile with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		t.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Chdir(startingDir)
		})

		// Use relative path to go back to repo root.
		cmd := atmosRunner.Command("--chdir", "../../..", "terraform", "generate", "varfile", "mycomponent", "--stack", "nonprod")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Stdout: %s", stdout.String())
			t.Logf("Stderr: %s", stderr.String())
		}

		assert.NotContains(t, stderr.String(), "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with relative --chdir path")
	})

	t.Run("terraform plan with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		t.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Chdir(startingDir)
		})

		// Use relative path to go back to repo root.
		cmd := atmosRunner.Command("--chdir", "../../..", "terraform", "plan", "mycomponent", "--stack", "nonprod")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Stdout: %s", stdout.String())
			t.Logf("Stderr: %s", stderr.String())
		}

		assert.NotContains(t, stderr.String(), "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with relative --chdir path for terraform plan")
	})
}

// TestChdirWithDescribeCommands tests that describe commands work properly with --chdir.
// These are known to work, so this serves as a control group.
func TestChdirWithDescribeCommands(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	startingDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		t.Chdir(startingDir)
	})

	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "basic")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	componentDir := filepath.Join(absFixturesPath, "components", "terraform", "mock")
	require.DirExists(t, componentDir)

	t.Run("describe config with absolute --chdir path", func(t *testing.T) {
		// Change to component directory.
		t.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Chdir(startingDir)
		})

		cmd := atmosRunner.Command("--chdir", absFixturesPath, "describe", "config")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()

		assert.NoError(t, err, "describe config should work with --chdir")
		assert.NotContains(t, stderr.String(), "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with --chdir for describe config")
	})

	t.Run("describe stacks with absolute --chdir path", func(t *testing.T) {
		// Change to component directory.
		t.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Chdir(startingDir)
		})

		cmd := atmosRunner.Command("--chdir", absFixturesPath, "describe", "stacks")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()

		assert.NoError(t, err, "describe stacks should work with --chdir")
		assert.NotContains(t, stderr.String(), "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with --chdir for describe stacks")
	})
}
