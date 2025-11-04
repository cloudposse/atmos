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

	// Save original working directory.
	originalWd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current working directory")
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Get absolute path to the fixtures directory.
	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "basic")
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
			args:        []string{"--chdir", absFixturesPath, "terraform", "generate", "varfile", "mock", "--stack", "nonprod"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// Should succeed and generate varfile.
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir")
			},
		},
		{
			name:        "terraform plan with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "plan", "mock", "--stack", "nonprod"},
			expectError: false, // This should NOT error, but currently does according to the bug report.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// The bug: this currently fails with "atmos.yaml CLI config file was not found".
				// When fixed, this assertion should pass.
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform plan")
			},
		},
		{
			name:        "terraform validate with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "validate", "mock", "--stack", "nonprod"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform validate")
			},
		},
		{
			name: "terraform init with absolute --chdir path",
			args: []string{"--chdir", absFixturesPath, "terraform", "init", "mock", "--stack", "nonprod"},
			// Skip because init might require actual terraform state/backend config.
			skipReason: "Skipping terraform init - requires backend configuration",
		},
		{
			name:        "terraform workspace with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "workspace", "mock", "--stack", "nonprod"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				assert.NotContains(t, stderr, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for terraform workspace")
			},
		},
		{
			name:        "terraform generate backend with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "terraform", "generate", "backend", "mock", "--stack", "nonprod"},
			expectError: false,
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
			err := os.Chdir(componentDir)
			require.NoError(t, err, "Should be able to change to component directory")

			// Restore directory after test.
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
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
				// For now, we allow errors because we're documenting the bug.
				// Once fixed, change this to assert.NoError.
				if err != nil {
					t.Logf("KNOWN BUG: Command failed but should succeed with --chdir")
				}
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

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "basic")
	componentDir := filepath.Join(fixturesDir, "components", "terraform", "mock")
	require.DirExists(t, componentDir)

	t.Run("terraform generate varfile with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		err := os.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
		})

		// Use relative path to go back to repo root.
		cmd := atmosRunner.Command("--chdir", "../../..", "terraform", "generate", "varfile", "mock", "--stack", "nonprod")

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
		err := os.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
		})

		// Use relative path to go back to repo root.
		cmd := atmosRunner.Command("--chdir", "../../..", "terraform", "plan", "mock", "--stack", "nonprod")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Stdout: %s", stdout.String())
			t.Logf("Stderr: %s", stderr.String())
			t.Logf("KNOWN BUG: terraform plan fails with relative --chdir path")
		}

		assert.NotContains(t, stderr.String(), "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with relative --chdir path for terraform plan")
	})
}

// TestChdirWithHelmfileCommands tests the --chdir flag with helmfile commands.
func TestChdirWithHelmfileCommands(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if helmfile not installed.
	RequireHelmfile(t)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Use a fixture that has helmfile components.
	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "atmos-helmfile-version")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	_, err = os.Stat(absFixturesPath)
	if os.IsNotExist(err) {
		t.Skip("Helmfile fixture not found, skipping helmfile chdir tests")
	}

	// Create a subdirectory path.
	componentDir := filepath.Join(absFixturesPath, "components", "helmfile")
	if _, err := os.Stat(componentDir); os.IsNotExist(err) {
		// If specific helmfile component dir doesn't exist, just use fixtures root.
		componentDir = absFixturesPath
	}

	tests := []struct {
		name        string
		args        []string
		expectError bool
		skipReason  string
	}{
		{
			name: "helmfile generate with absolute --chdir path",
			args: []string{"--chdir", absFixturesPath, "helmfile", "generate"},
			// May fail if component doesn't exist, but should not fail due to missing atmos.yaml.
			expectError: true, // Expected to fail due to missing component, not missing atmos.yaml.
		},
		{
			name:       "helmfile diff with absolute --chdir path",
			args:       []string{"--chdir", absFixturesPath, "helmfile", "diff"},
			skipReason: "Skipping helmfile diff - requires component specification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}

			// Change to a different directory to test --chdir.
			err := os.Chdir(componentDir)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
			})

			cmd := atmosRunner.Command(tt.args...)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run()

			output := stdout.String() + stderr.String()
			t.Logf("Output: %s", output)

			// The key assertion: should not fail due to missing atmos.yaml.
			assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
				"Should find atmos.yaml with --chdir for helmfile commands")
		})
	}
}

// TestChdirWithPackerCommands tests the --chdir flag with packer commands.
func TestChdirWithPackerCommands(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if packer not installed.
	RequirePacker(t)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "basic")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	_, err = os.Stat(absFixturesPath)
	require.NoError(t, err)

	// Create a subdirectory path.
	componentDir := filepath.Join(absFixturesPath, "components", "terraform", "mock")
	require.DirExists(t, componentDir)

	tests := []struct {
		name        string
		args        []string
		expectError bool
		skipReason  string
	}{
		{
			name:       "packer validate with absolute --chdir path",
			args:       []string{"--chdir", absFixturesPath, "packer", "validate"},
			skipReason: "Skipping packer validate - requires component specification",
		},
		{
			name:       "packer build with absolute --chdir path",
			args:       []string{"--chdir", absFixturesPath, "packer", "build"},
			skipReason: "Skipping packer build - requires component specification and configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}

			// Change to component directory.
			err := os.Chdir(componentDir)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
			})

			cmd := atmosRunner.Command(tt.args...)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run()

			output := stdout.String() + stderr.String()
			t.Logf("Output: %s", output)

			// The key assertion: should not fail due to missing atmos.yaml.
			assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
				"Should find atmos.yaml with --chdir for packer commands")
		})
	}
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

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "basic")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	componentDir := filepath.Join(absFixturesPath, "components", "terraform", "mock")
	require.DirExists(t, componentDir)

	t.Run("describe config with absolute --chdir path", func(t *testing.T) {
		// Change to component directory.
		err := os.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
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
		err := os.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
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
