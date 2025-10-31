package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerraformWithNoAuthConfiguration reproduces the bug where running
// `atmos terraform plan` in CI (non-TTY) fails with "interactive identity selection requires a TTY"
// even when:
// - NO auth configuration exists
// - NO --identity flag provided
// - NO ATMOS_IDENTITY env var set
//
// This is the exact scenario reported by the user.
func TestTerraformWithNoAuthConfiguration(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name           string
		viperHasValue  bool
		viperValue     string
		setViper       bool
		expectTTYCheck bool
		description    string
	}{
		{
			name:           "no flag, viper returns empty string",
			viperHasValue:  false,
			setViper:       true,
			expectTTYCheck: false,
			description:    "When viper.GetString returns empty, should not trigger TTY check",
		},
		{
			name:           "no flag, viper returns __SELECT__ (BUG SCENARIO)",
			viperHasValue:  true,
			viperValue:     IdentityFlagSelectValue,
			setViper:       true,
			expectTTYCheck: true,
			description:    "BEFORE FIX: viper.GetString could return __SELECT__, triggering TTY check in CI",
		},
		{
			name:           "no flag, viper not queried at all (AFTER FIX)",
			viperHasValue:  false,
			setViper:       false,
			expectTTYCheck: false,
			description:    "AFTER FIX: Don't call viper.GetString when flag not changed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clean viper state
			viper.Reset()

			// Set up viper binding (mimics what cmd/auth.go does)
			err := viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			require.NoError(t, err)

			// Ensure no env vars are set (t.Setenv will auto-cleanup)
			// No need to explicitly unset

			// If test wants viper to have a value, set it explicitly
			if tc.viperHasValue {
				viper.Set(IdentityFlagName, tc.viperValue)
			}

			// Create command with identity flag (mimics terraform commands)
			cmd := &cobra.Command{
				Use: "plan",
			}
			cmd.Flags().StringP(IdentityFlagName, "i", "", "Specify identity")
			if identityFlag := cmd.Flags().Lookup(IdentityFlagName); identityFlag != nil {
				identityFlag.NoOptDefVal = IdentityFlagSelectValue
			}

			// Parse with NO --identity flag provided
			err = cmd.ParseFlags([]string{})
			require.NoError(t, err)

			// This simulates the OLD buggy code
			var identityFlag string
			if cmd.Flags().Changed(IdentityFlagName) {
				identityFlag, err = cmd.Flags().GetString(IdentityFlagName)
				require.NoError(t, err)
			} else if tc.setViper {
				// OLD CODE PATH: Falls back to viper
				identityFlag = viper.GetString(IdentityFlagName)
			}

			// Check if this would trigger forceSelect
			forceSelect := identityFlag == IdentityFlagSelectValue

			if tc.expectTTYCheck {
				assert.True(t, forceSelect, "Expected forceSelect to be true (would trigger TTY check in CI)")
				assert.Equal(t, IdentityFlagSelectValue, identityFlag, "identityFlag should be __SELECT__")
			} else {
				assert.False(t, forceSelect, "Expected forceSelect to be false (no TTY check)")
			}

			t.Logf("flags.Changed: %v, viperValue: '%s', identityFlag: '%s', forceSelect: %v",
				cmd.Flags().Changed(IdentityFlagName),
				viper.GetString(IdentityFlagName),
				identityFlag,
				forceSelect,
			)
		})
	}
}

// TestViperBehaviorWithNoValue tests what viper.GetString returns in different scenarios
// to understand the root cause of the bug.
func TestViperBehaviorWithNoValue(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(t *testing.T)
		expectedValue string
		description   string
	}{
		{
			name: "viper with BindEnv only, no env vars set",
			setup: func(t *testing.T) {
				viper.Reset()
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			},
			expectedValue: "",
			description:   "Should return empty string when no env vars set",
		},
		{
			name: "viper with explicit Set",
			setup: func(t *testing.T) {
				viper.Reset()
				viper.Set(IdentityFlagName, IdentityFlagSelectValue)
			},
			expectedValue: IdentityFlagSelectValue,
			description:   "Should return the set value",
		},
		{
			name: "viper with ATMOS_IDENTITY env var",
			setup: func(t *testing.T) {
				viper.Reset()
				t.Setenv("ATMOS_IDENTITY", "test-identity")
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			},
			expectedValue: "test-identity",
			description:   "Should read from ATMOS_IDENTITY env var",
		},
		{
			name: "viper with IDENTITY env var (fallback)",
			setup: func(t *testing.T) {
				viper.Reset()
				t.Setenv("IDENTITY", "fallback-identity")
				viper.BindEnv(IdentityFlagName, "ATMOS_IDENTITY", "IDENTITY")
			},
			expectedValue: "fallback-identity",
			description:   "Should fallback to IDENTITY env var when ATMOS_IDENTITY not set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)

			value := viper.GetString(IdentityFlagName)
			assert.Equal(t, tc.expectedValue, value, tc.description)
			t.Logf("viper.GetString(%q) = %q", IdentityFlagName, value)
		})
	}
}
