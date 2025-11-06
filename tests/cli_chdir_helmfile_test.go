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

// TestChdirWithHelmfileCommands tests the --chdir flag with various helmfile commands.
// This reproduces the issue reported where --chdir works for some commands but fails for others.
// Helmfile commands use DisableFlagParsing=true similar to terraform, which may cause the same issue.
func TestChdirWithHelmfileCommands(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for chdir helmfile commands test", "coverageEnabled", coverDir != "")
	}

	RequireHelmfile(t)

	// Get absolute path to the complete fixtures directory (has helmfile components).
	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "complete")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err, "Failed to get absolute path to fixtures")

	// Verify fixtures directory exists.
	_, err = os.Stat(absFixturesPath)
	require.NoError(t, err, "Fixtures directory should exist at %s", absFixturesPath)

	// Create a subdirectory to simulate running from a component directory.
	componentDir := filepath.Join(absFixturesPath, "components", "helmfile", "echo-server")
	require.DirExists(t, componentDir, "Component directory should exist")

	tests := []struct {
		name        string
		args        []string
		expectError bool
		skipReason  string
		checkOutput func(t *testing.T, stdout, stderr string)
	}{
		{
			name:        "helmfile generate varfile with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "generate", "varfile", "echo-server", "--stack", "ue2-dev"},
			expectError: false, // This should NOT error, but may fail like terraform plan does.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// The bug: this may fail with "atmos.yaml CLI config file was not found".
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for helmfile generate varfile")
			},
		},
		{
			name:        "helmfile diff with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "diff", "echo-server", "--stack", "ue2-dev"},
			skipReason:  "Skipping helmfile diff - requires kubernetes cluster connection",
			expectError: false,
		},
		{
			name:        "helmfile template with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "template", "echo-server", "--stack", "ue2-dev"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for helmfile template")
			},
		},
		{
			name:        "helmfile lint with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "lint", "echo-server", "--stack", "ue2-dev"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for helmfile lint")
			},
		},
		{
			name:        "helmfile sync with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "sync", "echo-server", "--stack", "ue2-dev"},
			skipReason:  "Skipping helmfile sync - requires kubernetes cluster connection",
			expectError: false,
		},
		{
			name:        "helmfile apply with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "apply", "echo-server", "--stack", "ue2-dev"},
			skipReason:  "Skipping helmfile apply - requires kubernetes cluster connection",
			expectError: false,
		},
		{
			name:        "helmfile destroy with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "destroy", "echo-server", "--stack", "ue2-dev"},
			skipReason:  "Skipping helmfile destroy - requires kubernetes cluster connection",
			expectError: false,
		},
		{
			name:        "helmfile version with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "helmfile", "version"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// Version command should work regardless of directory.
				// This serves as a control to verify helmfile binary is accessible.
				output := stdout + stderr
				assert.Contains(t, output, "version",
					"Should output helmfile version information")
			},
		},
	}

	//nolint:dupl // Test code duplication is acceptable for clarity
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipReason != "" {
				t.Skip(tt.skipReason)
			}

			// Change to component directory to simulate CI/CD behavior.
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
				// Command is expected to fail (but not due to missing atmos.yaml).
				if err == nil {
					t.Logf("Command succeeded unexpectedly")
				}
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

// TestChdirWithHelmfileRelativePaths tests --chdir with relative paths for helmfile commands.
func TestChdirWithHelmfileRelativePaths(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	RequireHelmfile(t)

	startingDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		t.Chdir(startingDir)
	})

	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "complete")
	componentDir := filepath.Join(fixturesDir, "components", "helmfile", "echo-server")
	require.DirExists(t, componentDir)

	t.Run("helmfile generate with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		t.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Chdir(startingDir)
		})

		// Use relative path to go back to repo root (../../.. from components/helmfile/echo-server).
		cmd := atmosRunner.Command("--chdir", "../../..", "helmfile", "generate", "varfile", "echo-server", "--stack", "ue2-dev")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Stdout: %s", stdout.String())
			t.Logf("Stderr: %s", stderr.String())
		}

		output := stdout.String() + stderr.String()
		assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with relative --chdir path for helmfile generate")
	})

	t.Run("helmfile template with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		t.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			t.Chdir(startingDir)
		})

		// Use relative path to go back to repo root.
		cmd := atmosRunner.Command("--chdir", "../../..", "helmfile", "template", "echo-server", "--stack", "ue2-dev")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Stdout: %s", stdout.String())
			t.Logf("Stderr: %s", stderr.String())
		}

		output := stdout.String() + stderr.String()
		assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with relative --chdir path for helmfile template")
	})
}

// TestChdirWithHelmfileFromDifferentDirectory tests helmfile commands when run from various directories.
// This simulates real-world scenarios where CI/CD systems may start in different locations.
func TestChdirWithHelmfileFromDifferentDirectory(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	RequireHelmfile(t)

	startingDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		t.Chdir(startingDir)
	})

	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "complete")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	scenarios := []struct {
		name      string
		startDir  string
		chdirPath string
	}{
		{
			name:      "from components/helmfile directory",
			startDir:  filepath.Join(absFixturesPath, "components", "helmfile"),
			chdirPath: "../..",
		},
		{
			name:      "from stacks directory",
			startDir:  filepath.Join(absFixturesPath, "stacks"),
			chdirPath: "..",
		},
		{
			name:      "from nested component directory",
			startDir:  filepath.Join(absFixturesPath, "components", "helmfile", "echo-server"),
			chdirPath: "../../..",
		},
		{
			name:      "from infra component subdirectory",
			startDir:  filepath.Join(absFixturesPath, "components", "helmfile", "infra", "infra-server"),
			chdirPath: "../../../..",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Change to the scenario's start directory.
			t.Chdir(scenario.startDir)
			require.NoError(t, err, "Should be able to change to start directory")
			t.Cleanup(func() {
				t.Chdir(startingDir)
			})

			// Run helmfile generate varfile with --chdir.
			cmd := atmosRunner.Command("--chdir", scenario.chdirPath, "helmfile", "generate", "varfile", "echo-server", "--stack", "ue2-dev")

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run()

			output := stdout.String() + stderr.String()
			assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
				"Should find atmos.yaml with --chdir from %s", scenario.name)
		})
	}
}

// TestChdirWithHelmfileMultipleComponents tests --chdir with different helmfile components.
// This ensures the bug affects multiple components consistently.
func TestChdirWithHelmfileMultipleComponents(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	RequireHelmfile(t)

	startingDir, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		t.Chdir(startingDir)
	})

	fixturesDir := filepath.Join(startingDir, "fixtures", "scenarios", "complete")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	// Test from a subdirectory.
	componentDir := filepath.Join(absFixturesPath, "components", "helmfile")
	require.DirExists(t, componentDir)

	t.Chdir(componentDir)
	require.NoError(t, err)
	t.Cleanup(func() {
		t.Chdir(startingDir)
	})

	components := []struct {
		name  string
		stack string
	}{
		{name: "echo-server", stack: "ue2-dev"},
		{name: "infra/infra-server", stack: "ue2-dev"},
	}

	for _, comp := range components {
		t.Run("helmfile_generate_varfile_"+comp.name, func(t *testing.T) {
			cmd := atmosRunner.Command("--chdir", "../..", "helmfile", "generate", "varfile", comp.name, "--stack", comp.stack)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			_ = cmd.Run()

			output := stdout.String() + stderr.String()
			assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
				"Should find atmos.yaml with --chdir for component %s", comp.name)
		})
	}
}
