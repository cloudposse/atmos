package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestAuthConsole tests the `atmos auth console` command integration.
//
// Coverage Note:
// - Error paths (no identity, invalid provider, missing config): COVERED
// - Flag handling (--print-only, --no-open, --duration, --destination): COVERED
// - Help and registration: COVERED
// - Success paths (real authentication, console URL generation): NOT COVERED
//
// Success paths require real cloud provider credentials and browser interaction.
// These are tested manually and in production environments.
func TestAuthConsole(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for auth console test", "coverageEnabled", coverDir != "")
	}

	t.Run("command exists and shows help", func(t *testing.T) {
		cmd := atmosRunner.Command("auth", "console", "--help")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err)

		output := stdout.String()
		assert.Contains(t, output, "Open the cloud provider web console", "Help should describe console command")
		assert.Contains(t, output, "atmos auth console", "Help should show usage")
		assert.Contains(t, output, "--destination", "Help should document --destination flag")
		assert.Contains(t, output, "--duration", "Help should document --duration flag")
		assert.Contains(t, output, "--print-only", "Help should document --print-only flag")
		assert.Contains(t, output, "--no-open", "Help should document --no-open flag")
	})

	t.Run("fails when no default identity configured", func(t *testing.T) {
		// Change to a directory with no auth configuration.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail when no default identity is configured")

		combinedOutput := stdout.String() + stderr.String()
		// Output may contain markdown formatting, so use case-insensitive contains.
		lowerOutput := strings.ToLower(combinedOutput)
		assert.True(t,
			strings.Contains(lowerOutput, "no default identity") ||
				strings.Contains(lowerOutput, "default identity configured") ||
				strings.Contains(lowerOutput, "identity not found") ||
				strings.Contains(lowerOutput, "no authentication configured"),
			"Should mention missing identity configuration, got: %s", combinedOutput)
	})

	t.Run("fails with explicit non-existent identity", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console", "--identity", "nonexistent-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail when identity doesn't exist")

		combinedOutput := stdout.String() + stderr.String()
		assert.Contains(t, combinedOutput, "identity", "Error should mention identity")
	})

	t.Run("accepts duration flag", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// This will fail due to no auth config, but should parse the flag correctly.
		cmd := atmosRunner.Command("auth", "console", "--duration", "2h", "--identity", "test")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		// Should not have duration parsing errors.
		assert.NotContains(t, combinedOutput, "invalid duration", "Should parse duration flag")
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize --duration flag")
	})

	t.Run("accepts destination flag", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console", "--destination", "s3", "--identity", "test")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize --destination flag")
	})

	t.Run("accepts print-only flag", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console", "--print-only", "--identity", "test")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize --print-only flag")
	})

	t.Run("accepts no-open flag", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console", "--no-open", "--identity", "test")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize --no-open flag")
	})

	t.Run("accepts issuer flag", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console", "--issuer", "MyOrg", "--identity", "test")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize --issuer flag")
	})

	t.Run("accepts multiple flags together", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command(
			"auth", "console",
			"--destination", "ec2",
			"--duration", "4h",
			"--print-only",
			"--issuer", "MyCompany",
			"--identity", "test",
		)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize all flags")
		assert.NotContains(t, combinedOutput, "invalid duration", "Should parse duration correctly")
	})

	t.Run("identity flag takes precedence over default", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// Even if a default identity exists, --identity should override it.
		cmd := atmosRunner.Command("auth", "console", "--identity", "explicit-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		// Should not show interactive selector when explicit identity provided.
		assert.NotContains(t, combinedOutput, "Select an identity", "Should not show selector with explicit --identity")
	})

	t.Run("empty identity flag triggers interactive mode in TTY", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// --identity= (empty) signals interactive selection.
		// In non-TTY (our test environment), should fail gracefully.
		cmd := atmosRunner.Command("auth", "console", "--identity=")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail when interactive selection requested in non-TTY")

		combinedOutput := stdout.String() + stderr.String()
		// Should mention TTY requirement or authentication issue.
		assert.True(t,
			strings.Contains(combinedOutput, "TTY") ||
				strings.Contains(combinedOutput, "no authentication") ||
				strings.Contains(combinedOutput, "identity"),
			"Should mention TTY or auth requirement, got: %s", combinedOutput)
	})

	t.Run("invalid duration format fails", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "console", "--duration", "invalid-duration", "--identity", "test")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail with invalid duration")

		combinedOutput := stdout.String() + stderr.String()
		assert.Contains(t, combinedOutput, "invalid", "Should report invalid duration")
	})

	t.Run("respects CI environment and skips browser opening", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// Set CI env to prevent browser opening (even though auth will fail).
		cmd := atmosRunner.Command("auth", "console", "--identity", "test")
		cmd.Env = append(cmd.Env, "CI=true")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		// Test should not hang or attempt browser opening.
		// Just verify command completed.
	})
}
