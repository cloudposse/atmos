package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags/preprocess"
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

// TestParsePagerFlag_WithInheritedFlags tests that parsePagerFlag correctly handles
// persistent flags inherited from parent commands.
// This is a regression test for the bug where pager flag didn't work on subcommands.
func TestParsePagerFlag_WithInheritedFlags(t *testing.T) {
	tests := []struct {
		name             string
		isSubcommand     bool
		flagValue        *string // nil = don't set
		expectedProvided bool
		expectedValue    string
		expectedEnabled  bool
	}{
		{
			name:             "root command - pager enabled",
			isSubcommand:     false,
			flagValue:        strPtr("true"),
			expectedProvided: true,
			expectedValue:    "true",
			expectedEnabled:  true,
		},
		{
			name:             "root command - pager disabled",
			isSubcommand:     false,
			flagValue:        strPtr("false"),
			expectedProvided: true,
			expectedValue:    "false",
			expectedEnabled:  false,
		},
		{
			name:             "root command - pager not provided",
			isSubcommand:     false,
			flagValue:        nil,
			expectedProvided: false,
			expectedValue:    "",
			expectedEnabled:  false,
		},
		{
			name:             "subcommand - pager enabled (inherited)",
			isSubcommand:     true,
			flagValue:        strPtr("true"),
			expectedProvided: true,
			expectedValue:    "true",
			expectedEnabled:  true,
		},
		{
			name:             "subcommand - pager disabled (inherited)",
			isSubcommand:     true,
			flagValue:        strPtr("false"),
			expectedProvided: true,
			expectedValue:    "false",
			expectedEnabled:  false,
		},
		{
			name:             "subcommand - custom pager name (inherited)",
			isSubcommand:     true,
			flagValue:        strPtr("less"),
			expectedProvided: true,
			expectedValue:    "less",
			expectedEnabled:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create root command with persistent pager flag.
			rootCmd := &cobra.Command{Use: "root"}
			rootCmd.PersistentFlags().String("pager", "", "Enable pager")

			var testCmd *cobra.Command
			if tt.isSubcommand {
				// Create subcommand that inherits the flag.
				subCmd := &cobra.Command{Use: "sub"}
				rootCmd.AddCommand(subCmd)
				testCmd = subCmd
			} else {
				testCmd = rootCmd
			}

			// Set flag value if specified.
			if tt.flagValue != nil {
				var err error
				if tt.isSubcommand {
					// For subcommands, set inherited flags via InheritedFlags().
					err = testCmd.InheritedFlags().Set("pager", *tt.flagValue)
				} else {
					// For root command, use PersistentFlags().
					err = testCmd.PersistentFlags().Set("pager", *tt.flagValue)
				}
				require.NoError(t, err)
			}

			// Create Viper instance and bind the flag.
			v := viper.New()
			flag := testCmd.Flags().Lookup("pager")
			if flag == nil && !tt.isSubcommand {
				// For root command without value set, check persistent flags.
				flag = testCmd.PersistentFlags().Lookup("pager")
			}
			if tt.flagValue != nil {
				require.NotNil(t, flag, "pager flag should exist (local or inherited)")
			}
			if flag != nil {
				_ = v.BindPFlag("pager", flag)
			}

			// Parse the pager flag.
			result := parsePagerFlag(testCmd, v)

			// Verify results.
			assert.Equal(t, tt.expectedProvided, result.IsProvided(), "IsProvided()")
			assert.Equal(t, tt.expectedValue, result.Value(), "Value()")
			assert.Equal(t, tt.expectedEnabled, result.IsEnabled(), "IsEnabled()")
		})
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}

// TestParseIdentityFlag_WithInheritedFlags tests that parseIdentityFlag correctly handles
// persistent flags inherited from parent commands.
func TestParseIdentityFlag_WithInheritedFlags(t *testing.T) {
	tests := []struct {
		name             string
		isSubcommand     bool
		flagValue        *string // nil = don't set
		expectedProvided bool
		expectedValue    string
	}{
		{
			name:             "root command - identity provided",
			isSubcommand:     false,
			flagValue:        strPtr("prod-admin"),
			expectedProvided: true,
			expectedValue:    "prod-admin",
		},
		{
			name:             "root command - identity not provided",
			isSubcommand:     false,
			flagValue:        nil,
			expectedProvided: false,
			expectedValue:    "",
		},
		{
			name:             "subcommand - identity provided (inherited)",
			isSubcommand:     true,
			flagValue:        strPtr("dev-user"),
			expectedProvided: true,
			expectedValue:    "dev-user",
		},
		{
			name:             "subcommand - interactive selector (inherited)",
			isSubcommand:     true,
			flagValue:        strPtr("__SELECT__"),
			expectedProvided: true,
			expectedValue:    "__SELECT__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create root command with persistent identity flag.
			rootCmd := &cobra.Command{Use: "root"}
			rootCmd.PersistentFlags().String("identity", "", "Identity selector")

			var testCmd *cobra.Command
			if tt.isSubcommand {
				subCmd := &cobra.Command{Use: "sub"}
				rootCmd.AddCommand(subCmd)
				testCmd = subCmd
			} else {
				testCmd = rootCmd
			}

			// Set flag value if specified.
			if tt.flagValue != nil {
				var err error
				if tt.isSubcommand {
					err = testCmd.InheritedFlags().Set("identity", *tt.flagValue)
				} else {
					err = testCmd.PersistentFlags().Set("identity", *tt.flagValue)
				}
				require.NoError(t, err)
			}

			// Create Viper instance and bind the flag.
			v := viper.New()
			flag := testCmd.Flags().Lookup("identity")
			if flag == nil && !tt.isSubcommand {
				flag = testCmd.PersistentFlags().Lookup("identity")
			}
			if tt.flagValue != nil {
				require.NotNil(t, flag, "identity flag should exist (local or inherited)")
			}
			if flag != nil {
				_ = v.BindPFlag("identity", flag)
			}

			// Parse the identity flag.
			result := parseIdentityFlag(testCmd, v)

			// Verify results.
			assert.Equal(t, tt.expectedProvided, result.IsProvided(), "IsProvided()")
			assert.Equal(t, tt.expectedValue, result.Value(), "Value()")
		})
	}
}

// TestParseGlobalFlags_Integration tests the full ParseGlobalFlags function
// with a realistic command hierarchy.
func TestParseGlobalFlags_Integration(t *testing.T) {
	// Create root command with global flags.
	rootCmd := &cobra.Command{Use: "atmos"}
	globalBuilder := NewGlobalOptionsBuilder()
	parser := globalBuilder.Build()
	parser.RegisterPersistentFlags(rootCmd)

	// Create subcommand.
	subCmd := &cobra.Command{Use: "toolchain"}
	rootCmd.AddCommand(subCmd)

	// Create sub-subcommand.
	listCmd := &cobra.Command{Use: "list"}
	subCmd.AddCommand(listCmd)

	// Set pager flag on the list command (inherited from root).
	// Use InheritedFlags() to set inherited flags.
	err := listCmd.InheritedFlags().Set("pager", "false")
	require.NoError(t, err)

	// Create Viper and bind flags.
	v := viper.New()
	err = parser.BindToViper(v)
	require.NoError(t, err)

	// Bind the changed flag to Viper.
	flag := listCmd.Flags().Lookup("pager")
	require.NotNil(t, flag, "pager flag should be inherited")
	err = v.BindPFlag("pager", flag)
	require.NoError(t, err)

	// Parse global flags.
	flags := ParseGlobalFlags(listCmd, v)

	// Verify pager flag was parsed correctly.
	assert.True(t, flags.Pager.IsProvided(), "Pager should be provided")
	assert.Equal(t, "false", flags.Pager.Value(), "Pager value should be 'false'")
	assert.False(t, flags.Pager.IsEnabled(), "Pager should be disabled")
}

// TestParsePagerFlag_FallbackToEnv tests that parsePagerFlag falls back to
// environment variables when the flag is not provided.
func TestParsePagerFlag_FallbackToEnv(t *testing.T) {
	// Create command with pager flag.
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("pager", "", "Enable pager")

	// Create Viper and set env var.
	v := viper.New()
	v.Set("pager", "less") // Simulate env var.

	// Parse without setting the flag.
	result := parsePagerFlag(cmd, v)

	// Should use env var value.
	assert.True(t, result.IsProvided(), "Should be provided from env")
	assert.Equal(t, "less", result.Value(), "Should use env value")
	assert.True(t, result.IsEnabled(), "Should be enabled")
}

// TestParsePagerFlag_NoFlagRegistered tests behavior when pager flag is not registered.
func TestParsePagerFlag_NoFlagRegistered(t *testing.T) {
	// Create command WITHOUT pager flag.
	cmd := &cobra.Command{Use: "test"}

	// Create Viper.
	v := viper.New()

	// Parse - should return not provided.
	result := parsePagerFlag(cmd, v)

	assert.False(t, result.IsProvided(), "Should not be provided")
	assert.Equal(t, "", result.Value(), "Value should be empty")
	assert.False(t, result.IsEnabled(), "Should not be enabled")
}

func TestGlobalFlagsRegistry_ContainsNoOptDefValFlags(t *testing.T) {
	registry := GlobalFlagsRegistry()

	// Verify identity flag is registered with NoOptDefVal.
	identityFlag := registry.Get("identity")
	assert.NotNil(t, identityFlag, "identity flag should be registered")
	assert.Equal(t, cfg.IdentityFlagSelectValue, identityFlag.GetNoOptDefVal(), "identity should have NoOptDefVal set")
	assert.Equal(t, "i", identityFlag.GetShorthand(), "identity should have shorthand 'i'")

	// Verify pager flag is registered with NoOptDefVal.
	pagerFlag := registry.Get("pager")
	assert.NotNil(t, pagerFlag, "pager flag should be registered")
	assert.Equal(t, "true", pagerFlag.GetNoOptDefVal(), "pager should have NoOptDefVal set")
}

func TestGlobalFlagsRegistry_PreprocessesIdentityFlag(t *testing.T) {
	registry := GlobalFlagsRegistry()

	// Convert flags to preprocess.FlagInfo interface.
	allFlags := registry.All()
	flagInfos := make([]preprocess.FlagInfo, len(allFlags))
	for i, f := range allFlags {
		flagInfos[i] = f
	}

	// Create the preprocessor.
	preprocessor := preprocess.NewNoOptDefValPreprocessor(flagInfos)

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "identity with space-separated value",
			input:    []string{"auth", "login", "--identity", "prod-admin"},
			expected: []string{"auth", "login", "--identity=prod-admin"},
		},
		{
			name:     "identity shorthand with space-separated value",
			input:    []string{"auth", "login", "-i", "prod-admin"},
			expected: []string{"auth", "login", "-i=prod-admin"},
		},
		{
			name:     "identity with equals syntax unchanged",
			input:    []string{"auth", "login", "--identity=prod-admin"},
			expected: []string{"auth", "login", "--identity=prod-admin"},
		},
		{
			name:     "identity at end unchanged",
			input:    []string{"auth", "login", "--identity"},
			expected: []string{"auth", "login", "--identity"},
		},
		{
			name:     "identity followed by another flag unchanged",
			input:    []string{"auth", "login", "--identity", "--verbose"},
			expected: []string{"auth", "login", "--identity", "--verbose"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := preprocessor.Preprocess(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
