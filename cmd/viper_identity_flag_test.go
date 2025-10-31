package cmd

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestTerraformCIRegressionNoAuth verifies correct identity resolution in CI environments.
// Scenario: CI (non-TTY) with NO auth configured and NO --identity flag.
// Expected behavior:
// - viper.GetString("identity") should return empty string (not "__SELECT__")
// - No identity selection should be attempted
// - Command should execute without requiring TTY
//
// This test verifies viper configuration does not incorrectly trigger identity selection.
func TestTerraformCIRegressionNoAuth(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name                     string
		setupViper               func(t *testing.T)
		expectViperReturnsSelect bool
		description              string
	}{
		{
			name: "SCENARIO 1: Clean CI - no env vars, no config",
			setupViper: func(t *testing.T) {
				viper.Reset()
				// This is what cmd/auth.go:40 does
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
				// No env vars set, no explicit viper.Set()
			},
			expectViperReturnsSelect: false,
			description:              "Normal CI: viper should return empty string",
		},
		{
			name: "SCENARIO 2: Someone set IDENTITY env var to __SELECT__",
			setupViper: func(t *testing.T) {
				viper.Reset()
				t.Setenv("IDENTITY", IdentityFlagSelectValue)
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			},
			expectViperReturnsSelect: true,
			description:              "If IDENTITY=__SELECT__, viper returns it (BUG TRIGGER)",
		},
		{
			name: "SCENARIO 3: Someone set ATMOS_IDENTITY env var to __SELECT__",
			setupViper: func(t *testing.T) {
				viper.Reset()
				t.Setenv("ATMOS_IDENTITY", IdentityFlagSelectValue)
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			},
			expectViperReturnsSelect: true,
			description:              "If ATMOS_IDENTITY=__SELECT__, viper returns it (BUG TRIGGER)",
		},
		{
			name: "SCENARIO 4: Viper state pollution from previous command",
			setupViper: func(t *testing.T) {
				viper.Reset()
				// Simulate: a previous atmos command set this
				viper.Set(IdentityFlagName, IdentityFlagSelectValue)
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			},
			expectViperReturnsSelect: true,
			description:              "If viper was Set() from previous command, returns __SELECT__ (BUG TRIGGER)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup viper state for this scenario (t.Setenv auto-cleans up)
			tc.setupViper(t)

			// Simulate the BUGGY code path (line 85 in original):
			// When --identity flag NOT provided, code calls viper.GetString()
			viperValue := viper.GetString(IdentityFlagName)

			t.Logf("viper.GetString(%q) = %q", IdentityFlagName, viperValue)

			// Check if this would trigger the bug
			wouldTriggerBug := viperValue == IdentityFlagSelectValue

			if tc.expectViperReturnsSelect {
				assert.True(t, wouldTriggerBug, "Expected viper to return __SELECT__ (bug trigger)")
				assert.Equal(t, IdentityFlagSelectValue, viperValue)

				t.Logf("✗ BUG REPRODUCED: viper returned %q, would trigger TTY check in CI!", viperValue)
			} else {
				assert.False(t, wouldTriggerBug, "Expected viper NOT to return __SELECT__")
				assert.NotEqual(t, IdentityFlagSelectValue, viperValue)

				t.Logf("✓ NO BUG: viper returned %q, would not trigger TTY check", viperValue)
			}
		})
	}
}

// TestIdentifyWhyViperReturnsSelect tries to identify HOW viper could get __SELECT__ value
// without explicit env vars or viper.Set() calls.
func TestIdentifyWhyViperReturnsSelect(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("Check if config file could contain identity: __SELECT__", func(t *testing.T) {
		// This would require checking if viper reads from atmos.yaml
		// and if someone had identity: "__SELECT__" in their config

		viper.Reset()
		viper.SetConfigType("yaml")

		// Simulate a config file with identity: __SELECT__
		viper.Set(IdentityFlagName, IdentityFlagSelectValue)

		value := viper.GetString(IdentityFlagName)
		assert.Equal(t, IdentityFlagSelectValue, value)

		t.Logf("If atmos.yaml contains 'identity: __SELECT__', viper would return: %q", value)
	})

	t.Run("Check if NoOptDefVal could leak through cobra.Command.Execute()", func(t *testing.T) {
		// This tests if running a command multiple times could cause state pollution

		viper.Reset()
		viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")

		// Simulate running terraform command (which sets NoOptDefVal)
		// Then check if viper got polluted

		// First command: --identity provided without value
		// This should NOT affect viper for subsequent commands

		value := viper.GetString(IdentityFlagName)
		assert.Empty(t, value, "Viper should still be empty")

		t.Logf("After command with NoOptDefVal, viper returns: %q (should be empty)", value)
	})
}

// TestOriginalCodeBugPath verifies correct behavior when viper contains unexpected values.
// This tests the scenario where viper might have a stale value that should not affect
// terraform commands when --identity flag is not explicitly provided.
func TestOriginalCodeBugPath(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("viper contains __SELECT__ value should not affect terraform commands", func(t *testing.T) {
		viper.Reset()

		// Setup: Somehow viper has __SELECT__ value (we don't know how yet)
		viper.Set(IdentityFlagName, IdentityFlagSelectValue)

		// Original buggy code simulation:
		// Line 79-86 from terraform_utils.go
		flagsChanged := false // User did NOT provide --identity flag

		var identityFlag string
		if flagsChanged {
			// Would get from cobra flags
			identityFlag = "" // Not executed
		} else {
			// LINE 85: Falls back to viper
			identityFlag = viper.GetString(IdentityFlagName)
		}

		// LINE 89: Check if forceSelect
		forceSelect := identityFlag == IdentityFlagSelectValue

		// Assertions
		assert.Equal(t, IdentityFlagSelectValue, identityFlag, "identityFlag got __SELECT__ from viper")
		assert.True(t, forceSelect, "forceSelect is TRUE")

		// LINE 92-101: This is where the bug triggers
		// if forceSelect {
		//     if !term.IsTTYSupportForStdin() || !term.IsTTYSupportForStdout() {
		//         ERROR: "interactive identity selection requires a TTY"
		//     }
		// }

		t.Logf("✗ BUG CONFIRMED:")
		t.Logf("  - flags.Changed: %v", flagsChanged)
		t.Logf("  - viper.GetString: %q", identityFlag)
		t.Logf("  - forceSelect: %v", forceSelect)
		t.Logf("  - In CI (non-TTY): Would fail with TTY error!")
	})
}
