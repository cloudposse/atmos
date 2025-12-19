package tests

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestSkipInitFlag tests that the --skip-init flag prevents terraform init from running.
// This test addresses DEV-3847: Atmos version 1.202.0 ignores --skip-init for terraform output.
func TestSkipInitFlag(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	assert.NoError(t, err)
	err = os.Unsetenv("ATMOS_BASE_PATH")
	assert.NoError(t, err)

	// Use the basic fixture which has a simple terraform component.
	workDir := "fixtures/scenarios/basic"
	t.Chdir(workDir)

	// First, run terraform apply to set up the state so we can run output later.
	t.Run("setup_apply", func(t *testing.T) {
		cmd := atmosRunner.Command("terraform", "apply", "mycomponent", "-s", "prod")
		envVars := os.Environ()
		envVars = append(envVars, "ATMOS_COMPONENTS_TERRAFORM_APPLY_AUTO_APPROVE=true")
		cmd.Env = envVars

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			t.Logf("Setup apply stdout:\n%s", stdout.String())
			t.Logf("Setup apply stderr:\n%s", stderr.String())
			t.Fatalf("Failed to setup terraform apply: %v", err)
		}
	})

	// Test 1: Without --skip-init, terraform output should show "Initializing the backend..."
	t.Run("without_skip_init_shows_initializing", func(t *testing.T) {
		cmd := atmosRunner.Command("terraform", "output", "mycomponent", "-s", "prod")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Combine stdout and stderr for checking output.
		combinedOutput := stdout.String() + stderr.String()

		t.Logf("Without --skip-init stdout:\n%s", stdout.String())
		t.Logf("Without --skip-init stderr:\n%s", stderr.String())

		require.NoError(t, err, "terraform output without --skip-init should succeed")
		assert.Contains(t, combinedOutput, "Initializing the backend",
			"Without --skip-init, output should contain 'Initializing the backend'")
	})

	// Test 2: With --skip-init, terraform output should NOT show "Initializing the backend..."
	t.Run("with_skip_init_skips_initializing", func(t *testing.T) {
		cmd := atmosRunner.Command("terraform", "output", "mycomponent", "-s", "prod", "--skip-init")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Combine stdout and stderr for checking output.
		combinedOutput := stdout.String() + stderr.String()

		t.Logf("With --skip-init stdout:\n%s", stdout.String())
		t.Logf("With --skip-init stderr:\n%s", stderr.String())

		require.NoError(t, err, "terraform output with --skip-init should succeed")
		assert.NotContains(t, combinedOutput, "Initializing the backend",
			"With --skip-init, output should NOT contain 'Initializing the backend'")

		// Verify we still got the expected output values.
		assert.True(t, strings.Contains(combinedOutput, "foo") || strings.Contains(combinedOutput, "bar"),
			"Output should still contain the terraform output values")
	})

	// Cleanup: run terraform clean to remove state files.
	t.Run("cleanup", func(t *testing.T) {
		cmd := atmosRunner.Command("terraform", "clean", "--force")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		_ = cmd.Run() // Ignore error for cleanup.
	})
}
