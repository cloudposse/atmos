package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestAuthLoginProvider tests the `atmos auth login --provider <provider>` command integration.
//
// Coverage Note:
// - Error paths (no provider config, invalid provider, missing config): COVERED
// - Flag handling (--provider, -p, combined with --identity): COVERED
// - Help and registration: COVERED
// - Success paths (real authentication, credential storage): NOT COVERED
//
// Success paths require real cloud provider credentials and are tested manually and in production environments.
func TestAuthLoginProvider(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for auth login provider test", "coverageEnabled", coverDir != "")
	}

	t.Run("command shows help with provider flag", func(t *testing.T) {
		cmd := atmosRunner.Command("auth", "login", "--help")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.NoError(t, err)

		output := stdout.String()
		assert.Contains(t, output, "atmos auth login", "Help should show usage")
		assert.Contains(t, output, "--provider", "Help should document --provider flag")
		assert.Contains(t, output, "-p,", "Help should show short flag -p")
		assert.Contains(t, output, "auto-provisioning", "Help should describe provider flag purpose")
	})

	t.Run("fails when provider doesn't exist", func(t *testing.T) {
		// Change to a directory with auth configuration.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--provider", "nonexistent-provider")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail when provider doesn't exist")

		combinedOutput := stdout.String() + stderr.String()
		lowerOutput := strings.ToLower(combinedOutput)
		assert.True(t,
			strings.Contains(lowerOutput, "provider") ||
				strings.Contains(lowerOutput, "not found") ||
				strings.Contains(lowerOutput, "nonexistent-provider"),
			"Should mention provider issue, got: %s", combinedOutput)
	})

	t.Run("accepts provider flag with long form", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--provider", "test-provider")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		// Should not have flag parsing errors.
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize --provider flag")
		assert.NotContains(t, combinedOutput, "flag needs an argument", "Should accept provider value")
	})

	t.Run("accepts provider flag with short form", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "-p", "test-provider")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should recognize -p flag")
		assert.NotContains(t, combinedOutput, "flag needs an argument", "Should accept provider value")
	})

	t.Run("provider flag takes precedence over identity flag", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// When both --provider and --identity are specified, --provider should take precedence.
		cmd := atmosRunner.Command("auth", "login", "--provider", "test-provider", "--identity", "test-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		// Should attempt provider authentication, not identity authentication.
		// The error should mention the provider, not the identity.
		lowerOutput := strings.ToLower(combinedOutput)
		assert.True(t,
			strings.Contains(lowerOutput, "provider") ||
				strings.Contains(lowerOutput, "test-provider"),
			"Should attempt provider authentication when --provider is specified, got: %s", combinedOutput)
	})

	t.Run("rejects empty provider value", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--provider", "")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail with empty provider value")

		combinedOutput := stdout.String() + stderr.String()
		lowerOutput := strings.ToLower(combinedOutput)
		assert.True(t,
			strings.Contains(lowerOutput, "provider") ||
				strings.Contains(lowerOutput, "authentication") ||
				strings.Contains(lowerOutput, "not found"),
			"Should report issue with empty provider, got: %s", combinedOutput)
	})

	t.Run("works without provider flag for identity auth", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// Without --provider, should fall back to identity-based authentication.
		cmd := atmosRunner.Command("auth", "login", "--identity", "test-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		lowerOutput := strings.ToLower(combinedOutput)
		// Should attempt identity authentication.
		assert.True(t,
			strings.Contains(lowerOutput, "identity") ||
				strings.Contains(lowerOutput, "authentication"),
			"Should attempt identity authentication without --provider, got: %s", combinedOutput)
	})

	t.Run("fails gracefully when no auth configuration exists", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("auth", "login", "--provider", "aws-sso")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err, "Should fail when no auth configuration exists")

		combinedOutput := stdout.String() + stderr.String()
		// Should provide helpful error message.
		lowerOutput := strings.ToLower(combinedOutput)
		assert.True(t,
			strings.Contains(lowerOutput, "provider") ||
				strings.Contains(lowerOutput, "authentication") ||
				strings.Contains(lowerOutput, "configuration") ||
				strings.Contains(lowerOutput, "not found"),
			"Should provide helpful error message, got: %s", combinedOutput)
	})

	t.Run("provider flag with special characters", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// Test provider names with hyphens, underscores, dots.
		testCases := []string{
			"my-provider",
			"my_provider",
			"my.provider",
			"provider-123",
		}

		for _, providerName := range testCases {
			t.Run(providerName, func(t *testing.T) {
				cmd := atmosRunner.Command("auth", "login", "--provider", providerName)

				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr

				err := cmd.Run()
				require.Error(t, err) // Will fail due to missing auth config.

				combinedOutput := stdout.String() + stderr.String()
				// Should not have flag parsing errors.
				assert.NotContains(t, combinedOutput, "unknown flag", "Should accept provider name: "+providerName)
			})
		}
	})

	t.Run("respects CI environment", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// Set CI env to ensure non-interactive behavior.
		cmd := atmosRunner.Command("auth", "login", "--provider", "test-provider")
		cmd.Env = append(cmd.Env, "CI=true")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		// Test should not hang or attempt interactive prompts.
		// Just verify command completed.
	})

	t.Run("combines provider flag with global flags", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		// Test that --provider works with global flags like --no-color.
		cmd := atmosRunner.Command("auth", "login", "--provider", "test", "--no-color")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		require.Error(t, err) // Will fail due to missing auth config.

		combinedOutput := stdout.String() + stderr.String()
		assert.NotContains(t, combinedOutput, "unknown flag", "Should accept combined flags")
	})
}
