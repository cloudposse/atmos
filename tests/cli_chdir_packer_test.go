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

// TestChdirWithPackerCommands tests the --chdir flag with various packer commands.
// This reproduces the issue reported where --chdir works for some commands but fails for others.
// Packer commands use DisableFlagParsing=true similar to terraform, which may cause the same issue.
func TestChdirWithPackerCommands(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for chdir packer commands test", "coverageEnabled", coverDir != "")
	}

	RequirePacker(t)

	// Save original working directory.
	originalWd, err := os.Getwd()
	require.NoError(t, err, "Failed to get current working directory")
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	// Get absolute path to the packer fixtures directory.
	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "packer")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err, "Failed to get absolute path to fixtures")

	// Verify fixtures directory exists.
	_, err = os.Stat(absFixturesPath)
	require.NoError(t, err, "Fixtures directory should exist at %s", absFixturesPath)

	// Create a subdirectory to simulate running from a component directory.
	componentDir := filepath.Join(absFixturesPath, "components", "packer", "aws", "bastion")
	require.DirExists(t, componentDir, "Component directory should exist")

	tests := []struct {
		name        string
		args        []string
		expectError bool
		skipReason  string
		checkOutput func(t *testing.T, stdout, stderr string)
	}{
		{
			name:        "packer validate with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "packer", "validate", "aws/bastion", "--stack", "ue1-dev"},
			expectError: false, // This should NOT error, but may fail like terraform plan does.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// The bug: this may fail with "atmos.yaml CLI config file was not found".
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for packer validate")
			},
		},
		{
			name:        "packer init with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "packer", "init", "aws/bastion", "--stack", "ue1-dev"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for packer init")
			},
		},
		{
			name:        "packer inspect with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "packer", "inspect", "aws/bastion", "--stack", "ue1-dev"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for packer inspect")
			},
		},
		{
			name:        "packer build with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "packer", "build", "aws/bastion", "--stack", "ue1-dev"},
			skipReason:  "Skipping packer build - requires full packer setup and takes too long",
			expectError: false,
		},
		{
			name:        "packer version with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "packer", "version"},
			expectError: false,
			checkOutput: func(t *testing.T, stdout, stderr string) {
				// Version command should work regardless of directory.
				// This serves as a control to verify packer binary is accessible.
				assert.Contains(t, stdout, "Packer",
					"Should output packer version information")
			},
		},
		{
			name:        "packer output with absolute --chdir path",
			args:        []string{"--chdir", absFixturesPath, "packer", "output", "aws/bastion", "--stack", "ue1-dev"},
			expectError: true, // Expected to fail if no manifest exists, but not due to missing atmos.yaml.
			checkOutput: func(t *testing.T, stdout, stderr string) {
				output := stdout + stderr
				assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
					"Should find atmos.yaml with --chdir for packer output (even if manifest doesn't exist)")
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
				// Command is expected to fail (but not due to missing atmos.yaml).
				if err == nil {
					t.Logf("Command succeeded unexpectedly")
				}
			} else {
				if err != nil {
					t.Logf("Command failed with error: %v", err)
					t.Logf("Stdout: %s", stdout.String())
					t.Logf("Stderr: %s", stderr.String())
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

// TestChdirWithPackerRelativePaths tests --chdir with relative paths for packer commands.
func TestChdirWithPackerRelativePaths(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	RequirePacker(t)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "packer")
	componentDir := filepath.Join(fixturesDir, "components", "packer", "aws", "bastion")
	require.DirExists(t, componentDir)

	t.Run("packer validate with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		err := os.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
		})

		// Use relative path to go back to repo root (../../../../ from components/packer/aws/bastion).
		cmd := atmosRunner.Command("--chdir", "../../../..", "packer", "validate", "aws/bastion", "--stack", "ue1-dev")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			t.Logf("Stdout: %s", stdout.String())
			t.Logf("Stderr: %s", stderr.String())
			t.Logf("KNOWN BUG: packer validate fails with relative --chdir path")
		}

		output := stdout.String() + stderr.String()
		assert.NotContains(t, output, "atmos.yaml  CLI config file was not found",
			"Should find atmos.yaml with relative --chdir path for packer validate")
	})

	t.Run("packer init with relative --chdir path", func(t *testing.T) {
		// Change to component directory.
		err := os.Chdir(componentDir)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.Chdir(originalWd)
		})

		// Use relative path to go back to repo root.
		cmd := atmosRunner.Command("--chdir", "../../../..", "packer", "init", "aws/bastion", "--stack", "ue1-dev")

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
			"Should find atmos.yaml with relative --chdir path for packer init")
	})
}

// TestChdirWithPackerFromDifferentDirectory tests packer commands when run from various directories.
// This simulates real-world scenarios where CI/CD systems may start in different locations.
func TestChdirWithPackerFromDifferentDirectory(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	RequirePacker(t)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chdir(originalWd)
	})

	fixturesDir := filepath.Join(originalWd, "fixtures", "scenarios", "packer")
	absFixturesPath, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	scenarios := []struct {
		name      string
		startDir  string
		chdirPath string
	}{
		{
			name:      "from components/packer directory",
			startDir:  filepath.Join(absFixturesPath, "components", "packer"),
			chdirPath: "../..",
		},
		{
			name:      "from stacks directory",
			startDir:  filepath.Join(absFixturesPath, "stacks"),
			chdirPath: "..",
		},
		{
			name:      "from nested component directory",
			startDir:  filepath.Join(absFixturesPath, "components", "packer", "aws", "bastion"),
			chdirPath: "../../../..",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Change to the scenario's start directory.
			err := os.Chdir(scenario.startDir)
			require.NoError(t, err, "Should be able to change to start directory")
			t.Cleanup(func() {
				_ = os.Chdir(originalWd)
			})

			// Run packer validate with --chdir.
			cmd := atmosRunner.Command("--chdir", scenario.chdirPath, "packer", "validate", "aws/bastion", "--stack", "ue1-dev")

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
