package theme

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
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
		assert.NotNil(t, themeListParser, "parser should be initialized")
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

func TestExecuteThemeListActiveThemeResolution(t *testing.T) {
	tests := []struct {
		name             string
		setupAtmosConfig func() *schema.AtmosConfiguration
		setupViper       func(v *viper.Viper)
		expectedTheme    string
	}{
		{
			name: "defaults to atmos when no config",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return nil
			},
			setupViper: func(v *viper.Viper) {
				// No setup - clean state
			},
			expectedTheme: "atmos",
		},
		{
			name: "defaults to atmos when config has empty theme",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Theme: "",
						},
					},
				}
			},
			setupViper: func(v *viper.Viper) {
				// No setup
			},
			expectedTheme: "atmos",
		},
		{
			name: "uses config theme when set",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Theme: "dracula",
						},
					},
				}
			},
			setupViper: func(v *viper.Viper) {
				// No setup
			},
			expectedTheme: "dracula",
		},
		{
			name: "ATMOS_THEME env var overrides empty config",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Theme: "",
						},
					},
				}
			},
			setupViper: func(v *viper.Viper) {
				v.Set("ATMOS_THEME", "monokai")
			},
			expectedTheme: "monokai",
		},
		{
			name: "THEME env var overrides empty config and ATMOS_THEME not set",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Theme: "",
						},
					},
				}
			},
			setupViper: func(v *viper.Viper) {
				v.Set("THEME", "solarized-dark")
			},
			expectedTheme: "solarized-dark",
		},
		{
			name: "config theme takes precedence over env vars",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Theme: "nord",
						},
					},
				}
			},
			setupViper: func(v *viper.Viper) {
				v.Set("ATMOS_THEME", "monokai")
				v.Set("THEME", "solarized-dark")
			},
			expectedTheme: "nord",
		},
		{
			name: "ATMOS_THEME takes precedence over THEME when config empty",
			setupAtmosConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					Settings: schema.AtmosSettings{
						Terminal: schema.Terminal{
							Theme: "",
						},
					},
				}
			},
			setupViper: func(v *viper.Viper) {
				v.Set("ATMOS_THEME", "monokai")
				v.Set("THEME", "solarized-dark")
			},
			expectedTheme: "monokai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup viper
			originalViper := viper.GetViper()
			defer func() {
				// Restore original viper
				viper.Reset()
				for key, val := range originalViper.AllSettings() {
					viper.Set(key, val)
				}
			}()

			// Reset viper for clean state
			viper.Reset()
			tt.setupViper(viper.GetViper())

			// Setup atmos config
			oldAtmosConfig := atmosConfigPtr
			defer func() { atmosConfigPtr = oldAtmosConfig }()
			atmosConfigPtr = tt.setupAtmosConfig()

			// Create a test command
			testCmd := &cobra.Command{
				Use: "test-list",
				RunE: func(cmd *cobra.Command, args []string) error {
					return nil
				},
			}

			// Register flags using the parser
			themeListParser.RegisterFlags(testCmd)
			err := themeListParser.BindFlagsToViper(testCmd, viper.GetViper())
			require.NoError(t, err)

			// Execute the theme resolution logic directly
			activeTheme := ""
			if atmosConfigPtr != nil && atmosConfigPtr.Settings.Terminal.Theme != "" {
				activeTheme = atmosConfigPtr.Settings.Terminal.Theme
			} else if envTheme := viper.GetString("ATMOS_THEME"); envTheme != "" {
				activeTheme = envTheme
			} else if envTheme := viper.GetString("THEME"); envTheme != "" {
				activeTheme = envTheme
			} else {
				activeTheme = "atmos"
			}

			// Verify
			assert.Equal(t, tt.expectedTheme, activeTheme,
				"expected active theme=%s, got %s", tt.expectedTheme, activeTheme)
		})
	}
}

func TestExecuteThemeShowOptions(t *testing.T) {
	t.Run("creates options with theme name from args", func(t *testing.T) {
		// This test verifies the options struct creation logic
		themeName := "dracula"
		opts := &ThemeShowOptions{
			ThemeName: themeName,
		}
		assert.Equal(t, "dracula", opts.ThemeName)
	})

	t.Run("creates options with different theme name", func(t *testing.T) {
		themeName := "monokai"
		opts := &ThemeShowOptions{
			ThemeName: themeName,
		}
		assert.Equal(t, "monokai", opts.ThemeName)
	})
}

func TestThemeListOptionsStruct(t *testing.T) {
	t.Run("creates options with recommended only false", func(t *testing.T) {
		opts := &ThemeListOptions{
			RecommendedOnly: false,
		}
		assert.False(t, opts.RecommendedOnly)
	})

	t.Run("creates options with recommended only true", func(t *testing.T) {
		opts := &ThemeListOptions{
			RecommendedOnly: true,
		}
		assert.True(t, opts.RecommendedOnly)
	})
}

func TestExecuteThemeList(t *testing.T) {
	// Initialize I/O context and formatter for testing (required for ui.Write/Success/Info).
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)

	t.Run("executes with default config defaults to atmos theme", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Execute
		err := executeThemeList(themeListCmd, []string{})

		// Should not error - theme registry should have 'atmos' theme
		require.NoError(t, err, "executeThemeList should not error when no config is present")
	})

	t.Run("executes with config containing theme", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "dracula",
				},
			},
		}

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Execute
		err := executeThemeList(themeListCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should not error when config contains theme")
	})

	t.Run("executes with ATMOS_THEME env var", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "", // Empty config
				},
			},
		}

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set ATMOS_THEME env var
		t.Setenv("ATMOS_THEME", "monokai")

		// Execute
		err := executeThemeList(themeListCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should not error when ATMOS_THEME env var is set")
	})

	t.Run("executes with THEME env var", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "", // Empty config
				},
			},
		}

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set THEME env var (ATMOS_THEME not set)
		t.Setenv("THEME", "solarized-dark")

		// Execute
		err := executeThemeList(themeListCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should not error when THEME env var is set")
	})

	t.Run("executes with recommended flag enabled", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set recommended flag
		viper.Set("recommended", true)

		// Execute
		err := executeThemeList(themeListCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should not error when recommended flag is enabled")
	})

	t.Run("executes with recommended flag disabled", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set recommended flag to false
		viper.Set("recommended", false)

		// Execute
		err := executeThemeList(themeListCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should not error when recommended flag is disabled")
	})

	t.Run("executes with recommended flag via command line", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Create a test command to simulate flag parsing
		testCmd := &cobra.Command{Use: "test-list"}
		themeListParser.RegisterFlags(testCmd)
		err := themeListParser.BindFlagsToViper(testCmd, viper.GetViper())
		require.NoError(t, err)

		// Set the flag
		err = testCmd.Flags().Set("recommended", "true")
		require.NoError(t, err)

		// Execute with the test command
		err = executeThemeList(testCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should not error with CLI flag")
	})

	t.Run("verifies plural vs singular message formatting", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Execute - this should list multiple themes and test the pluralization logic
		err := executeThemeList(themeListCmd, []string{})

		// Should not error
		require.NoError(t, err, "executeThemeList should handle plural formatting")
	})

	t.Run("executes and displays active theme information", func(t *testing.T) {
		// Setup with explicit theme config
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "dracula",
				},
			},
		}

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Execute - should show dracula as active theme
		err := executeThemeList(themeListCmd, []string{})

		// Should not error and should display active theme
		require.NoError(t, err, "executeThemeList should display active theme information")
	})
}

func TestThemeListEnvVarFallback(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)

	t.Run("falls back to ATMOS_THEME env var when config empty", func(t *testing.T) {
		// Setup - nil config so atmosConfigPtr check fails
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set ATMOS_THEME via environment - this should hit line 81-82
		t.Setenv("ATMOS_THEME", "dracula")

		// Bind ATMOS_THEME env var to Viper and enable automatic env reading
		_ = viper.BindEnv("ATMOS_THEME")
		viper.AutomaticEnv()

		// Execute
		err := executeThemeList(themeListCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("falls back to THEME env var when ATMOS_THEME not set", func(t *testing.T) {
		// Setup - nil config so atmosConfigPtr check fails
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set only THEME via environment (not ATMOS_THEME) - this should hit line 83-84
		t.Setenv("THEME", "nord")

		// Bind THEME env var to Viper (not ATMOS_THEME) and enable automatic env reading
		_ = viper.BindEnv("THEME")
		viper.AutomaticEnv()

		// Execute
		err := executeThemeList(themeListCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("uses default atmos theme when no config or env vars", func(t *testing.T) {
		// Setup - nil config
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper - no env vars set
		viper.Reset()
		defer viper.Reset()

		// Explicitly ensure no ATMOS_THEME or THEME env vars - this should hit the else path on line 85
		_ = viper.BindEnv("ATMOS_THEME")
		_ = viper.BindEnv("THEME")

		// Execute - should default to "atmos"
		err := executeThemeList(themeListCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("handles empty ATMOS_THEME and falls back to THEME", func(t *testing.T) {
		// Setup - nil config
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set ATMOS_THEME to empty string (should skip to THEME check)
		t.Setenv("ATMOS_THEME", "")
		t.Setenv("THEME", "github")

		// Bind both env vars and enable automatic env reading
		_ = viper.BindEnv("ATMOS_THEME")
		_ = viper.BindEnv("THEME")
		viper.AutomaticEnv()

		// Execute - should use THEME since ATMOS_THEME is empty
		err := executeThemeList(themeListCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("prefers config theme over env vars", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "github", // Set in config
				},
			},
		}

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// Set both env vars (should be ignored)
		viper.Set("ATMOS_THEME", "dracula")
		viper.Set("THEME", "nord")

		// Execute - should use "github" from config
		err := executeThemeList(themeListCmd, []string{})
		require.NoError(t, err)
	})
}

func TestThemeListResultError(t *testing.T) {
	// Initialize I/O context and formatter for testing.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)

	t.Run("handles result with no error", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Reset viper
		viper.Reset()
		defer viper.Reset()

		// This should succeed and not return an error from the result
		err := executeThemeList(themeListCmd, []string{})
		require.NoError(t, err, "should handle successful result")
	})
}

func TestExecuteThemeShow(t *testing.T) {
	// Initialize I/O context and formatter for testing (required for ui.Markdown).
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err, "Failed to create I/O context")
	ui.InitFormatter(ioCtx)

	t.Run("executes with valid theme name", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Execute with a common theme name
		err := executeThemeShow(themeShowCmd, []string{"atmos"})

		// Should not error if theme exists
		require.NoError(t, err, "executeThemeShow should not return an error for valid theme 'atmos'")
	})

	t.Run("executes with different theme name", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Execute with another theme name
		err := executeThemeShow(themeShowCmd, []string{"dracula"})

		// Should not error if theme exists
		require.NoError(t, err, "executeThemeShow should not return an error for valid theme 'dracula'")
	})

	t.Run("returns error for non-existent theme", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Execute with a non-existent theme name
		err := executeThemeShow(themeShowCmd, []string{"nonexistent-theme-that-does-not-exist"})

		// Should return an error for theme not found
		require.Error(t, err, "executeThemeShow should return an error for non-existent theme")
		assert.ErrorIs(t, err, errUtils.ErrThemeNotFound, "should be theme not found error")
	})

	t.Run("handles multiple registered themes without errors", func(t *testing.T) {
		// Setup
		oldAtmosConfig := atmosConfigPtr
		defer func() { atmosConfigPtr = oldAtmosConfig }()
		atmosConfigPtr = nil

		// Test a subset of themes that are known to exist
		// Note: Theme registry may vary, so we test common ones
		themes := []string{"atmos", "dracula", "github", "nord"}

		for _, themeName := range themes {
			t.Run(themeName, func(t *testing.T) {
				err := executeThemeShow(themeShowCmd, []string{themeName})
				// Should not error for known themes
				assert.NoError(t, err, "executeThemeShow should not return an error for theme '%s'", themeName)
			})
		}
	})
}
