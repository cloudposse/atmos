package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCobraNoOptDefValWithViper tests if Cobra's NoOptDefVal can affect viper.GetString
// when the flag is NOT explicitly provided by the user.
//
// This test investigates if the NoOptDefVal setting could somehow leak into viper's
// return value, which might explain the bug.
func TestCobraNoOptDefValWithViper(t *testing.T) {
	tests := []struct {
		name                 string
		setupViper           bool
		bindPFlag            bool
		args                 []string
		expectedFlagValue    string
		expectedViperValue   string
		expectedFlagsChanged bool
		description          string
	}{
		{
			name:                 "no flag provided, no viper binding",
			setupViper:           false,
			bindPFlag:            false,
			args:                 []string{},
			expectedFlagValue:    "",
			expectedViperValue:   "",
			expectedFlagsChanged: false,
			description:          "Normal case: no flag, no binding",
		},
		{
			name:                 "no flag provided, viper BindEnv only",
			setupViper:           true,
			bindPFlag:            false,
			args:                 []string{},
			expectedFlagValue:    "",
			expectedViperValue:   "",
			expectedFlagsChanged: false,
			description:          "Viper BindEnv does NOT affect flag value",
		},
		{
			name:                 "no flag provided, viper BindPFlag",
			setupViper:           true,
			bindPFlag:            true,
			args:                 []string{},
			expectedFlagValue:    "", // Should still be empty
			expectedViperValue:   "", // Should be empty since flag not provided
			expectedFlagsChanged: false,
			description:          "CRITICAL: Does BindPFlag cause NoOptDefVal to leak?",
		},
		{
			name:                 "flag without value, no viper binding",
			setupViper:           false,
			bindPFlag:            false,
			args:                 []string{"--identity"},
			expectedFlagValue:    "__SELECT__", // NoOptDefVal kicks in
			expectedViperValue:   "",           // Viper not set up
			expectedFlagsChanged: true,
			description:          "Flag without value sets NoOptDefVal",
		},
		{
			name:                 "flag without value, with BindPFlag",
			setupViper:           true,
			bindPFlag:            true,
			args:                 []string{"--identity"},
			expectedFlagValue:    "__SELECT__",
			expectedViperValue:   "__SELECT__", // Should viper see this?
			expectedFlagsChanged: true,
			description:          "When flag provided without value, both should see __SELECT__",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset viper state
			viper.Reset()

			// Create a fresh command
			cmd := &cobra.Command{
				Use: "test",
			}

			// Add identity flag with NoOptDefVal
			cmd.Flags().StringP("identity", "i", "", "test identity flag")
			identityFlag := cmd.Flags().Lookup("identity")
			require.NotNil(t, identityFlag)
			identityFlag.NoOptDefVal = "__SELECT__"

			// Set up viper if requested
			if tc.setupViper {
				err := viper.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY")
				require.NoError(t, err)

				if tc.bindPFlag {
					// BindPFlag creates two-way binding between flag and viper
					err = viper.BindPFlag("identity", identityFlag)
					require.NoError(t, err)
				}
			}

			// Parse flags
			err := cmd.ParseFlags(tc.args)
			require.NoError(t, err)

			// Get values
			flagValue, err := cmd.Flags().GetString("identity")
			require.NoError(t, err)
			viperValue := viper.GetString("identity")
			flagsChanged := cmd.Flags().Changed("identity")

			// Assertions
			assert.Equal(t, tc.expectedFlagValue, flagValue, "Flag value mismatch")
			assert.Equal(t, tc.expectedViperValue, viperValue, "Viper value mismatch")
			assert.Equal(t, tc.expectedFlagsChanged, flagsChanged, "flags.Changed() mismatch")

			t.Logf("Results:")
			t.Logf("  flags.GetString: %q", flagValue)
			t.Logf("  viper.GetString: %q", viperValue)
			t.Logf("  flags.Changed:   %v", flagsChanged)
			t.Logf("  NoOptDefVal:     %q", identityFlag.NoOptDefVal)
		})
	}
}

// TestViperBindPFlagPollution tests if viper.BindPFlag could cause
// the NoOptDefVal to leak into viper even when flag is not provided.
func TestViperBindPFlagPollution(t *testing.T) {
	viper.Reset()

	// Simulate terraform command setup
	cmd := &cobra.Command{Use: "terraform"}
	cmd.PersistentFlags().StringP("identity", "i", "", "Identity flag")

	identityFlag := cmd.PersistentFlags().Lookup("identity")
	require.NotNil(t, identityFlag)
	identityFlag.NoOptDefVal = "__SELECT__"

	// Setup viper (mimicking real code)
	err := viper.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY")
	require.NoError(t, err)

	// What if someone called BindPFlag?
	// (Check if this happens in the codebase)
	// err = viper.BindPFlag("identity", identityFlag)
	// require.NoError(t, err)

	// Parse with NO flag provided
	err = cmd.ParseFlags([]string{})
	require.NoError(t, err)

	// Check values
	flagValue, err := cmd.Flags().GetString("identity")
	require.NoError(t, err)
	viperValue := viper.GetString("identity")
	flagsChanged := cmd.Flags().Changed("identity")

	// These should all be empty/false when flag not provided
	assert.Empty(t, flagValue, "Flag value should be empty when not provided")
	assert.Empty(t, viperValue, "Viper value should be empty when no env vars set")
	assert.False(t, flagsChanged, "flags.Changed should be false when not provided")

	t.Logf("When NO flag provided:")
	t.Logf("  flags.GetString: %q", flagValue)
	t.Logf("  viper.GetString: %q", viperValue)
	t.Logf("  flags.Changed:   %v", flagsChanged)
}
