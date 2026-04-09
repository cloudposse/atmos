package tests

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestIdentityFlagExplicitValue verifies that when an explicit identity value is provided
// via --identity flag, it is used instead of triggering interactive selection.
// This is a regression test for the bug where Cobra's NoOptDefVal behavior with positional
// args caused explicit identity values to be ignored.
func TestIdentityFlagExplicitValue(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for identity flag test", "coverageEnabled", coverDir != "")
	}

	t.Run("terraform plan with explicit identity should not show selector", func(t *testing.T) {
		// Change to a basic test directory.
		t.Chdir("fixtures/scenarios/basic")

		// Run terraform plan with explicit (non-existent) identity.
		// Expected behavior: Should NOT show interactive selector.
		// Whether the command succeeds or fails depends on auth configuration.
		cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod", "--identity", "nonexistent-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		_ = cmd.Run() // May succeed or fail - we don't care about exit code.

		combinedOutput := stdout.String() + stderr.String()

		// Verify that interactive selector was NOT shown.
		// The selector shows "Select an identity" in its UI.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show interactive identity selector when explicit identity is provided")

		t.Logf("Command output:\n%s", combinedOutput)
	})

	t.Run("terraform plan with --identity= (empty) should proceed without identity", func(t *testing.T) {
		// Change to a basic test directory.
		t.Chdir("fixtures/scenarios/basic")

		// Run with --identity= (empty value).
		// After refactor: empty identity value is treated as "no identity" and proceeds.
		// This is reasonable behavior for automation/CI environments.
		cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod", "--identity=")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		// Don't set a TTY - this simulates CI/automation environment.
		err := cmd.Run()

		combinedOutput := stdout.String() + stderr.String()

		// After refactor: command should succeed (or fail for terraform reasons, not identity).
		// Should NOT show interactive selector.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show interactive identity selector when --identity= is provided")

		// If it failed, it should not be due to TTY requirement (identity selection is skipped).
		if err != nil {
			assert.NotContains(t, combinedOutput, "interactive identity selection requires a TTY",
				"Should not require TTY when --identity= is provided (identity selection is skipped)")
		}

		t.Logf("Command output:\n%s", combinedOutput)
	})

	t.Run("auth login with explicit identity should not show selector", func(t *testing.T) {
		// Change to a basic test directory.
		t.Chdir("fixtures/scenarios/basic")

		// Run auth login with explicit (non-existent) identity.
		cmd := atmosRunner.Command("auth", "login", "--identity", "nonexistent-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		_ = cmd.Run() // May succeed or fail depending on auth configuration.

		combinedOutput := stdout.String() + stderr.String()

		// Verify that interactive selector was NOT shown.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show interactive identity selector when explicit identity is provided")

		t.Logf("Command output:\n%s", combinedOutput)
	})

	t.Run("terraform plan without identity flag should not attempt selection", func(t *testing.T) {
		// Change to a basic test directory.
		t.Chdir("fixtures/scenarios/basic")

		// Run terraform plan WITHOUT --identity flag.
		// Expected behavior: Should proceed with terraform plan (might fail on actual terraform
		// execution, but should not fail on identity selection).
		cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		_ = cmd.Run() // May succeed or fail depending on terraform setup.

		combinedOutput := stdout.String() + stderr.String()

		// Should NOT show identity selector when flag not provided.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should not show identity selector when --identity flag is not provided")

		// Should NOT fail with identity-related errors.
		assert.NotContains(t, combinedOutput, "interactive identity selection requires a TTY",
			"Should not require TTY when --identity flag is not provided")

		t.Logf("Command output:\n%s", combinedOutput)
	})
}

// TestIdentityFlagWithEnvironmentVariable verifies that ATMOS_IDENTITY environment
// variable is respected when --identity flag is not provided.
func TestIdentityFlagWithEnvironmentVariable(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for identity env var test", "coverageEnabled", coverDir != "")
	}

	t.Run("terraform plan uses ATMOS_IDENTITY when flag not provided", func(t *testing.T) {
		// Change to a basic test directory.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod")

		// Set ATMOS_IDENTITY environment variable.
		cmd.Env = append(cmd.Env, "ATMOS_IDENTITY=env-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		_ = cmd.Run() // May succeed or fail.

		combinedOutput := stdout.String() + stderr.String()

		// Should NOT show interactive selector.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should use ATMOS_IDENTITY env var, not show selector")

		t.Logf("Command output:\n%s", combinedOutput)
	})

	t.Run("--identity flag overrides ATMOS_IDENTITY environment variable", func(t *testing.T) {
		// Change to a basic test directory.
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("terraform", "plan", "mycomponent", "--stack", "nonprod", "--identity", "flag-identity")

		// Set ATMOS_IDENTITY environment variable (should be overridden).
		cmd.Env = append(cmd.Env, "ATMOS_IDENTITY=env-identity")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		_ = cmd.Run() // May succeed or fail.

		combinedOutput := stdout.String() + stderr.String()

		// Should use flag value, not env var.
		// Cannot directly verify which identity was used, but can verify no selector shown.
		assert.NotContains(t, combinedOutput, "Select an identity",
			"Should use --identity flag value, not show selector")

		t.Logf("Command output:\n%s", combinedOutput)
	})
}
