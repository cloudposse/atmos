package tests

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestDescribeCommandsWithIdentityFlag verifies that describe commands handle the --identity flag correctly.
// These tests cover the code paths where identity flag is parsed and CreateAuthManagerFromIdentity is called.
func TestDescribeCommandsWithIdentityFlag(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
		logger.Info("Atmos runner initialized for describe identity test", "coverageEnabled", coverDir != "")
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "describe component with non-existent identity should fail gracefully",
			args: []string{"describe", "component", "mycomponent", "--stack", "nonprod", "--identity", "nonexistent-identity"},
		},
		{
			name: "describe stacks with non-existent identity should fail gracefully",
			args: []string{"describe", "stacks", "--identity", "nonexistent-identity"},
		},
		{
			name: "describe affected with non-existent identity should fail gracefully",
			args: []string{"describe", "affected", "--identity", "nonexistent-identity"},
		},
		{
			name: "describe dependents with non-existent identity should fail gracefully",
			args: []string{"describe", "dependents", "mycomponent", "--stack", "nonprod", "--identity", "nonexistent-identity"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("fixtures/scenarios/basic")

			cmd := atmosRunner.Command(tt.args...)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			// Should fail when given non-existent identity.
			assert.Error(t, err, "Command should fail with non-existent identity")

			combinedOutput := stdout.String() + stderr.String()

			// Should not show interactive selector when explicit identity is provided.
			assert.NotContains(t, combinedOutput, "Select an identity",
				"Should not show interactive selector with explicit identity")
		})
	}

	t.Run("describe component without identity flag should work normally", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/atmos-include-yaml-function")

		cmd := atmosRunner.Command("describe", "component", "component-1", "--stack", "nonprod")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed (component exists in test fixtures).
		assert.NoError(t, err, "describe component without identity should succeed")
	})
}

// TestDescribeCommandsWithoutAuthWork verifies that commands work normally without --identity flag.
func TestDescribeCommandsWithoutAuthWork(t *testing.T) {
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	t.Run("describe stacks without auth should work", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("describe", "stacks")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed when no identity flag is provided.
		assert.NoError(t, err, "describe stacks without identity flag should succeed")
	})

	t.Run("list components without auth should work", func(t *testing.T) {
		t.Chdir("fixtures/scenarios/basic")

		cmd := atmosRunner.Command("list", "components")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		// Should succeed when no identity flag is provided.
		assert.NoError(t, err, "list components without identity flag should succeed")
	})
}
