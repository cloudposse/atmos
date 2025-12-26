package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestParseIdentityFlag_NormalizesDisabledValues(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "false lowercase should be normalized to disabled",
			envValue: "false",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "FALSE uppercase should be normalized to disabled",
			envValue: "FALSE",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "False mixed case should be normalized to disabled",
			envValue: "False",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "0 should be normalized to disabled",
			envValue: "0",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "no should be normalized to disabled",
			envValue: "no",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "NO should be normalized to disabled",
			envValue: "NO",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "off should be normalized to disabled",
			envValue: "off",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "OFF should be normalized to disabled",
			envValue: "OFF",
			expected: cfg.IdentityFlagDisabledValue,
		},
		{
			name:     "normal identity should be unchanged",
			envValue: "prod-admin",
			expected: "prod-admin",
		},
		{
			name:     "identity with dashes should be unchanged",
			envValue: "my-identity-name",
			expected: "my-identity-name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset Viper state for test isolation.
			v := viper.New()

			// Create a minimal command with identity flag.
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.Flags().StringP(identityFlagName, "i", "", "Identity flag")
			if identityFlag := cmd.Flags().Lookup(identityFlagName); identityFlag != nil {
				identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
			}

			// Set the identity value via Viper (simulates env var).
			v.Set(identityFlagName, tc.envValue)

			// Parse the identity flag.
			selector := parseIdentityFlag(cmd, v)

			// Verify the value is normalized correctly.
			assert.Equal(t, tc.expected, selector.Value(), "identity value should be normalized")
			assert.True(t, selector.IsProvided(), "identity should be marked as provided")
		})
	}
}

func TestParseIdentityFlag_NotProvided(t *testing.T) {
	// Reset Viper state.
	v := viper.New()

	// Create a minimal command with identity flag.
	cmd := &cobra.Command{
		Use: "test",
	}
	cmd.Flags().StringP(identityFlagName, "i", "", "Identity flag")

	// Don't set any value in Viper.
	selector := parseIdentityFlag(cmd, v)

	// Should return empty and not provided.
	assert.Equal(t, "", selector.Value())
	assert.False(t, selector.IsProvided())
}

func TestParseIdentityFlag_FlagChanged(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		expected  string
	}{
		{
			name:      "flag set to false should be normalized",
			flagValue: "false",
			expected:  cfg.IdentityFlagDisabledValue,
		},
		{
			name:      "flag set to identity name should be unchanged",
			flagValue: "my-identity",
			expected:  "my-identity",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset Viper state.
			v := viper.New()

			// Create a minimal command with identity flag.
			cmd := &cobra.Command{
				Use: "test",
			}
			cmd.Flags().StringP(identityFlagName, "i", "", "Identity flag")
			if identityFlag := cmd.Flags().Lookup(identityFlagName); identityFlag != nil {
				identityFlag.NoOptDefVal = cfg.IdentityFlagSelectValue
			}

			// Set the flag value directly (simulates --identity=value).
			err := cmd.Flags().Set(identityFlagName, tc.flagValue)
			assert.NoError(t, err)

			// Sync to Viper (as would happen in real command execution).
			v.Set(identityFlagName, tc.flagValue)

			// Parse the identity flag.
			selector := parseIdentityFlag(cmd, v)

			// Verify the value is normalized correctly.
			assert.Equal(t, tc.expected, selector.Value(), "identity value should be normalized")
			assert.True(t, selector.IsProvided(), "identity should be marked as provided")
		})
	}
}
