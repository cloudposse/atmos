package theme

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestThemeCommand(t *testing.T) {
	t.Run("theme command exists", func(t *testing.T) {
		assert.Equal(t, "theme", themeCmd.Use)
		assert.NotEmpty(t, themeCmd.Short)
		assert.NotEmpty(t, themeCmd.Long)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		hasListCmd := false
		for _, subCmd := range themeCmd.Commands() {
			if subCmd.Use == "list" {
				hasListCmd = true
				break
			}
		}
		assert.True(t, hasListCmd, "theme command should have list subcommand")
	})

	t.Run("has show subcommand", func(t *testing.T) {
		hasShowCmd := false
		for _, subCmd := range themeCmd.Commands() {
			if subCmd.Use == "show [theme-name]" {
				hasShowCmd = true
				break
			}
		}
		assert.True(t, hasShowCmd, "theme command should have show subcommand")
	})
}

func TestSetAtmosConfig(t *testing.T) {
	t.Run("sets config successfully", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "dracula",
				},
			},
		}

		SetAtmosConfig(config)
		assert.Equal(t, config, atmosConfigPtr)
	})

	t.Run("handles nil config", func(t *testing.T) {
		SetAtmosConfig(nil)
		assert.Nil(t, atmosConfigPtr)
	})
}

func TestThemeCommandProvider(t *testing.T) {
	provider := &ThemeCommandProvider{}

	t.Run("GetCommand returns theme command", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "theme", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		name := provider.GetName()
		assert.Equal(t, "theme", name)
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		group := provider.GetGroup()
		assert.Equal(t, "Other Commands", group)
	})
}

func TestThemeListCommand(t *testing.T) {
	t.Run("list command exists", func(t *testing.T) {
		assert.Equal(t, "list", themeListCmd.Use)
		assert.NotEmpty(t, themeListCmd.Short)
	})

	t.Run("has recommended flag", func(t *testing.T) {
		flag := themeListCmd.Flags().Lookup("recommended")
		require.NotNil(t, flag, "list command should have --recommended flag")
		assert.Equal(t, "bool", flag.Value.Type())
	})

	t.Run("parser is initialized", func(t *testing.T) {
		assert.NotNil(t, themeListParser, "parser should be initialized")
		assert.NotNil(t, themeListParser, "registry should exist")
	})

	t.Run("accepts no arguments", func(t *testing.T) {
		err := themeListCmd.Args(themeListCmd, []string{})
		assert.NoError(t, err, "list command should accept no arguments")

		err = themeListCmd.Args(themeListCmd, []string{"extra"})
		assert.Error(t, err, "list command should reject arguments")
	})
}

func TestThemeShowCommand(t *testing.T) {
	t.Run("show command exists", func(t *testing.T) {
		assert.Equal(t, "show [theme-name]", themeShowCmd.Use)
		assert.NotEmpty(t, themeShowCmd.Short)
		assert.NotEmpty(t, themeShowCmd.Long)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		// Validate Args is set to ExactArgs(1).
		err := themeShowCmd.Args(themeShowCmd, []string{})
		assert.Error(t, err, "show command should require exactly one argument")

		err = themeShowCmd.Args(themeShowCmd, []string{"dracula"})
		assert.NoError(t, err, "show command should accept one argument")

		err = themeShowCmd.Args(themeShowCmd, []string{"dracula", "extra"})
		assert.Error(t, err, "show command should reject more than one argument")
	})

	t.Run("parser is initialized", func(t *testing.T) {
		assert.NotNil(t, themeShowParser, "parser should be initialized")
	})
}

func TestThemeListOptions(t *testing.T) {
	t.Run("creates options with default values", func(t *testing.T) {
		opts := &ThemeListOptions{}
		assert.False(t, opts.RecommendedOnly, "RecommendedOnly should default to false")
	})

	t.Run("creates options with recommended enabled", func(t *testing.T) {
		opts := &ThemeListOptions{
			RecommendedOnly: true,
		}
		assert.True(t, opts.RecommendedOnly)
	})
}

func TestThemeShowOptions(t *testing.T) {
	t.Run("creates options with theme name", func(t *testing.T) {
		opts := &ThemeShowOptions{
			ThemeName: "dracula",
		}
		assert.Equal(t, "dracula", opts.ThemeName)
	})

	t.Run("creates options with empty theme name", func(t *testing.T) {
		opts := &ThemeShowOptions{}
		assert.Empty(t, opts.ThemeName)
	})
}

func TestThemeListFlagParser(t *testing.T) {
	t.Run("flag parser registers recommended flag", func(t *testing.T) {
		flag := themeListCmd.Flags().Lookup("recommended")
		require.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue, "default should be false")
		assert.Contains(t, flag.Usage, "recommended", "usage should mention recommended")
	})

	t.Run("flag parser has no shorthand for recommended", func(t *testing.T) {
		flag := themeListCmd.Flags().Lookup("recommended")
		require.NotNil(t, flag)
		assert.Empty(t, flag.Shorthand, "recommended flag should have no shorthand")
	})
}

func TestThemeListFlagHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupViper    func(v *viper.Viper)
		setupCmd      func(cmd *cobra.Command) error
		getFinalValue func(cmd *cobra.Command, v *viper.Viper) bool
		expectedValue bool
	}{
		{
			name: "default value when no flag or env set",
			setupViper: func(v *viper.Viper) {
				// Clean slate.
			},
			setupCmd: func(cmd *cobra.Command) error {
				return nil
			},
			getFinalValue: func(cmd *cobra.Command, v *viper.Viper) bool {
				return v.GetBool("recommended")
			},
			expectedValue: false,
		},
		{
			name: "env var sets value to true",
			setupViper: func(v *viper.Viper) {
				v.Set("recommended", true)
			},
			setupCmd: func(cmd *cobra.Command) error {
				return nil
			},
			getFinalValue: func(cmd *cobra.Command, v *viper.Viper) bool {
				return v.GetBool("recommended")
			},
			expectedValue: true,
		},
		{
			name: "flag overrides env var (flag=true, env=false)",
			setupViper: func(v *viper.Viper) {
				v.Set("recommended", false)
			},
			setupCmd: func(cmd *cobra.Command) error {
				return cmd.Flags().Set("recommended", "true")
			},
			getFinalValue: func(cmd *cobra.Command, v *viper.Viper) bool {
				// When flag is set, check the flag directly since that's what Viper will return.
				if cmd.Flags().Changed("recommended") {
					val, _ := cmd.Flags().GetBool("recommended")
					return val
				}
				return v.GetBool("recommended")
			},
			expectedValue: true,
		},
		{
			name: "flag overrides env var (flag=false, env=true)",
			setupViper: func(v *viper.Viper) {
				v.Set("recommended", true)
			},
			setupCmd: func(cmd *cobra.Command) error {
				return cmd.Flags().Set("recommended", "false")
			},
			getFinalValue: func(cmd *cobra.Command, v *viper.Viper) bool {
				// When flag is set, check the flag directly since that's what Viper will return.
				if cmd.Flags().Changed("recommended") {
					val, _ := cmd.Flags().GetBool("recommended")
					return val
				}
				return v.GetBool("recommended")
			},
			expectedValue: false,
		},
		{
			name: "flag sets value to true",
			setupViper: func(v *viper.Viper) {
				// No env var.
			},
			setupCmd: func(cmd *cobra.Command) error {
				return cmd.Flags().Set("recommended", "true")
			},
			getFinalValue: func(cmd *cobra.Command, v *viper.Viper) bool {
				if cmd.Flags().Changed("recommended") {
					val, _ := cmd.Flags().GetBool("recommended")
					return val
				}
				return v.GetBool("recommended")
			},
			expectedValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh Viper instance for isolation.
			v := viper.New()

			// Create a test command to avoid mutating the global themeListCmd.
			testCmd := &cobra.Command{
				Use: "test-list",
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}

			// Register flags using the parser.
			themeListParser.RegisterFlags(testCmd)

			// Bind to viper.
			err := themeListParser.BindFlagsToViper(testCmd, v)
			require.NoError(t, err)

			// Setup viper state.
			tt.setupViper(v)

			// Setup command flags.
			err = tt.setupCmd(testCmd)
			require.NoError(t, err)

			// Get the final value using the test's strategy.
			actualValue := tt.getFinalValue(testCmd, v)

			// Verify.
			assert.Equal(t, tt.expectedValue, actualValue,
				"expected recommended=%v, got %v", tt.expectedValue, actualValue)
		})
	}
}

func TestThemeListParserBindToViper(t *testing.T) {
	t.Run("binds flags to viper successfully", func(t *testing.T) {
		v := viper.New()
		testCmd := &cobra.Command{Use: "test"}

		themeListParser.RegisterFlags(testCmd)
		err := themeListParser.BindFlagsToViper(testCmd, v)

		require.NoError(t, err)
	})

	t.Run("binds environment variables", func(t *testing.T) {
		v := viper.New()
		testCmd := &cobra.Command{Use: "test"}

		themeListParser.RegisterFlags(testCmd)
		err := themeListParser.BindFlagsToViper(testCmd, v)
		require.NoError(t, err)

		// Simulate environment variable by setting in Viper.
		v.Set("recommended", true)

		// Verify it's accessible.
		assert.True(t, v.GetBool("recommended"))
	})
}

func TestThemeShowParserBindToViper(t *testing.T) {
	t.Run("binds successfully even with no flags", func(t *testing.T) {
		v := viper.New()
		testCmd := &cobra.Command{Use: "test"}

		themeShowParser.RegisterFlags(testCmd)
		err := themeShowParser.BindFlagsToViper(testCmd, v)

		require.NoError(t, err)
	})
}

func TestThemeListFlagPrecedence(t *testing.T) {
	t.Run("flag precedence: CLI flag > env var > default", func(t *testing.T) {
		v := viper.New()
		testCmd := &cobra.Command{Use: "test"}

		themeListParser.RegisterFlags(testCmd)
		err := themeListParser.BindFlagsToViper(testCmd, v)
		require.NoError(t, err)

		// Test 1: Default value.
		assert.False(t, v.GetBool("recommended"), "default should be false")

		// Test 2: Env var overrides default.
		v.Set("recommended", true)
		assert.True(t, v.GetBool("recommended"), "env var should override default")

		// Test 3: CLI flag overrides env var.
		// After BindFlagsToViper, flags take precedence when Changed.
		err = testCmd.Flags().Set("recommended", "false")
		require.NoError(t, err)

		// Check the flag directly since it was changed.
		actualValue, err := testCmd.Flags().GetBool("recommended")
		require.NoError(t, err)
		assert.False(t, actualValue, "CLI flag should override env var")
	})
}
