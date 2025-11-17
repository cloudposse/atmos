package tests

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestInteractiveIdentitySelection tests the interactive identity selection behavior
// with different stdin/stdout/CI configurations.
func TestInteractiveIdentitySelection(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for interactive test", "coverageEnabled", coverDir != "")
	}

	t.Run("explicit identity value should work even with piped output", func(t *testing.T) {
		// Scenario: build/atmos auth login --identity asd
		// Expected: Uses explicit identity "asd", does not show selector.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--identity", "explicit-test-identity")

		var stdout strings.Builder
		var stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Don't set up TTY - simulates piped output.
		_ = cmd.Run() // May succeed or fail depending on auth config.

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging - only runs if THIS subtest fails
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Should NOT show selector.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show selector when explicit identity provided")

		// Should mention the explicit identity (either "not found" or used successfully).
		assert.Contains(t, combinedOutput, "explicit-test-identity",
			"Should reference the explicit identity value")
	})

	t.Run("identity flag without value should fail gracefully in non-TTY", func(t *testing.T) {
		// Scenario: build/atmos auth login --identity < /dev/null (or NUL on Windows)
		// Expected: Fails with "no default identity" or TTY error, does not hang.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--identity")

		// Redirect stdin from null device to simulate < /dev/null.
		// Use os.DevNull for cross-platform compatibility (/dev/null on Unix, NUL on Windows).
		devNull, err := os.Open(os.DevNull)
		require.NoError(t, err, "Failed to open null device")
		defer devNull.Close()
		cmd.Stdin = devNull

		var stdout strings.Builder
		var stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()

		// Command should fail (not hang).
		require.Error(t, err, "Command should fail when stdin is redirected")

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging - only runs if THIS subtest fails
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Should fail with appropriate error, not show selector.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show selector when stdin is not a TTY")

		// Should mention either "no default identity" or authentication error.
		assert.True(t,
			strings.Contains(combinedOutput, "no default identity") ||
				strings.Contains(combinedOutput, "authentication") ||
				strings.Contains(combinedOutput, "identity"),
			"Should fail with identity-related error, got: %s", combinedOutput)
	})

	t.Run("identity flag without value should fail in CI environment", func(t *testing.T) {
		// Scenario: CI=true build/atmos auth login --identity
		// Expected: Fails immediately with "no default identity", does not attempt interaction.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--identity")

		// Set CI=true environment variable.
		cmd.Env = append(os.Environ(), "CI=true")

		var stdout strings.Builder
		var stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		// Command should fail immediately.
		require.Error(t, err, "Command should fail when CI=true")

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging - only runs if THIS subtest fails
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Should NOT show selector in CI.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show selector in CI environment")

		// Should fail with appropriate error.
		assert.True(t,
			strings.Contains(combinedOutput, "no default identity") ||
				strings.Contains(combinedOutput, "authentication") ||
				strings.Contains(combinedOutput, "requires a TTY"),
			"Should fail with identity error in CI, got: %s", combinedOutput)
	})

	t.Run("terraform with explicit identity should work even with piped output", func(t *testing.T) {
		// Scenario: build/atmos terraform plan vpc --stack test --identity myid
		// Expected: Uses explicit identity "myid", does not show selector.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod", "--identity", "explicit-terraform-identity")

		var stdout strings.Builder
		var stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Don't set up TTY - simulates piped output.
		_ = cmd.Run() // May succeed or fail.

		combinedOutput := stdout.String() + stderr.String()

		// Defer output logging - only runs if THIS subtest fails
		defer func() {
			if t.Failed() {
				t.Logf("\n=== Full output from failed test ===")
				t.Logf("Output (%d bytes):\n%s", len(combinedOutput), combinedOutput)
			}
		}()

		// Should NOT show selector.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show selector when explicit identity provided to terraform")
	})
}

// TestCIEnvironmentDetection tests that various CI environment variables properly
// disable interactive mode.
func TestCIEnvironmentDetection(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for CI detection test", "coverageEnabled", coverDir != "")
	}

	ciEnvironments := []struct {
		name string
		env  map[string]string
		desc string
	}{
		{
			name: "GitHub Actions",
			env:  map[string]string{"GITHUB_ACTIONS": "true"},
			desc: "GitHub Actions CI environment",
		},
		{
			name: "GitLab CI",
			env:  map[string]string{"GITLAB_CI": "true"},
			desc: "GitLab CI environment",
		},
		{
			name: "CircleCI",
			env:  map[string]string{"CIRCLECI": "true"},
			desc: "CircleCI environment",
		},
		{
			name: "Jenkins",
			env:  map[string]string{"JENKINS_URL": "http://jenkins.example.com"},
			desc: "Jenkins CI environment",
		},
		{
			name: "Travis CI",
			env:  map[string]string{"TRAVIS": "true"},
			desc: "Travis CI environment",
		},
		{
			name: "Buildkite",
			env:  map[string]string{"BUILDKITE": "true"},
			desc: "Buildkite CI environment",
		},
		{
			name: "Generic CI",
			env:  map[string]string{"CI": "true"},
			desc: "Generic CI=true environment",
		},
	}

	for _, ciEnv := range ciEnvironments {
		t.Run(ciEnv.name, func(t *testing.T) {
			t.Chdir("fixtures/scenarios/basic")

			cmd := atmosRunner.Command("auth", "login", "--identity")

			// Set CI environment variables.
			envVars := os.Environ()
			for key, value := range ciEnv.env {
				envVars = append(envVars, key+"="+value)
			}
			cmd.Env = envVars

			var stdout strings.Builder
			var stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()

			// Command should fail in CI.
			require.Error(t, err, "Command should fail in %s", ciEnv.desc)

			combinedOutput := stdout.String() + stderr.String()

			// Defer output logging - only runs if THIS subtest fails
			defer func() {
				if t.Failed() {
					t.Logf("\n=== Full output from failed test ===")
					t.Logf("%s output (%d bytes):\n%s", ciEnv.name, len(combinedOutput), combinedOutput)
				}
			}()

			// Should NOT show interactive selector in CI.
			assert.NotContains(t, combinedOutput, "Select an identity",
				"Should not show selector in %s", ciEnv.desc)

			// Should fail with appropriate error.
			assert.True(t,
				strings.Contains(combinedOutput, "no default identity") ||
					strings.Contains(combinedOutput, "authentication") ||
					strings.Contains(combinedOutput, "requires a TTY"),
				"Should fail with identity error in %s, got: %s", ciEnv.desc, combinedOutput)
		})
	}
}

// TestStdoutPipingDoesNotBlockInteraction documents that piping stdout
// should not block interactive prompts (if stdin is a TTY).
// Note: This is a documentation test - we cannot actually create a TTY in unit tests.
func TestStdoutPipingDoesNotBlockInteraction(t *testing.T) {
	// This test documents the expected behavior when stdout is piped but stdin is a TTY.
	//
	// Example command: build/atmos terraform plan --identity | cat
	//
	// Expected behavior:
	// - If stdin is a TTY (user can type), interactive prompt should work
	// - The TUI selector will still render (though may look odd in piped output)
	// - User can still select an identity via stdin
	//
	// Current implementation:
	// - isInteractive() only checks: (stdin is TTY) AND (not CI)
	// - Stdout is NOT checked
	// - This allows piping output while maintaining interactivity
	//
	// Why this is correct:
	// - Users may want to pipe/redirect output for logging
	// - As long as stdin is available, they can still interact
	// - CI detection prevents hanging in automated environments

	t.Log("Documented behavior: Piping stdout does not block interactive prompts")
	t.Log("  Command: build/atmos terraform plan --identity | cat")
	t.Log("  stdin: TTY (can type)")
	t.Log("  stdout: piped (to cat)")
	t.Log("  Expected: Selector shown, user can interact via stdin")
	t.Log("")
	t.Log("  Implementation: isInteractive() = (stdin TTY) AND (not CI)")
	t.Log("  Stdout is NOT checked by design")
}

// TestExplicitIdentityAlwaysWorks verifies that explicit identity values
// work in all environments (TTY, non-TTY, CI, piped).
func TestExplicitIdentityAlwaysWorks(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for explicit identity test", "coverageEnabled", coverDir != "")
	}

	scenarios := []struct {
		name     string
		setupCmd func(*testing.T, *exec.Cmd)
		desc     string
	}{
		{
			name: "with CI=true",
			setupCmd: func(t *testing.T, cmd *exec.Cmd) {
				cmd.Env = append(os.Environ(), "CI=true")
			},
			desc: "Explicit identity should work even in CI",
		},
		{
			name: "with stdin from /dev/null",
			setupCmd: func(t *testing.T, cmd *exec.Cmd) {
				// Use os.DevNull for cross-platform compatibility (/dev/null on Unix, NUL on Windows).
				devNull, err := os.Open(os.DevNull)
				require.NoError(t, err, "failed to open null device")
				cmd.Stdin = devNull
				t.Cleanup(func() {
					_ = devNull.Close()
				})
			},
			desc: "Explicit identity should work with redirected stdin",
		},
		{
			name: "with GitHub Actions CI",
			setupCmd: func(t *testing.T, cmd *exec.Cmd) {
				cmd.Env = append(os.Environ(), "GITHUB_ACTIONS=true")
			},
			desc: "Explicit identity should work in GitHub Actions",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Chdir("fixtures/scenarios/basic")

			cmd := atmosRunner.Command("auth", "login", "--identity", "my-explicit-identity")

			// Apply scenario setup.
			scenario.setupCmd(t, cmd)

			var stdout strings.Builder
			var stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			_ = cmd.Run() // May succeed or fail based on auth config.

			combinedOutput := stdout.String() + stderr.String()

			// Defer output logging - only runs if THIS subtest fails
			defer func() {
				if t.Failed() {
					t.Logf("\n=== Full output from failed test ===")
					t.Logf("%s output (%d bytes):\n%s", scenario.name, len(combinedOutput), combinedOutput)
				}
			}()

			// Should NOT show interactive selector.
			assert.NotContains(t, combinedOutput, "Select an identity",
				"%s: should not show selector with explicit identity", scenario.desc)

			// Should reference the explicit identity.
			assert.Contains(t, combinedOutput, "my-explicit-identity",
				"%s: should use explicit identity value", scenario.desc)
		})
	}
}
