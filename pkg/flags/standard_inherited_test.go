package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestStandardFlagParser_InheritedPersistentFlags tests that persistent flags
// from parent commands are correctly parsed when DisableFlagParsing=true.
// This is a critical test for the fix in commit b03bfa210.
func TestStandardFlagParser_InheritedPersistentFlags(t *testing.T) {
	tests := []struct {
		name               string
		parentFlags        []string // Persistent flags on parent
		childFlags         []string // Local flags on child
		args               []string // Command line args
		expectedPersist    map[string]string
		expectedLocal      map[string]string
		expectedPositional []string
		expectError        bool
	}{
		{
			name:        "persistent flag from parent is recognized",
			parentFlags: []string{"logs-level"},
			childFlags:  []string{"component"},
			args:        []string{"--logs-level=Debug", "--component=vpc"},
			expectedPersist: map[string]string{
				"logs-level": "Debug",
			},
			expectedLocal: map[string]string{
				"component": "vpc",
			},
			expectedPositional: []string{},
		},
		{
			name:        "persistent flag mixed with positional args",
			parentFlags: []string{"logs-level"},
			childFlags:  []string{"stack"},
			args:        []string{"component-name", "--logs-level=Debug", "--stack=prod"},
			expectedPersist: map[string]string{
				"logs-level": "Debug",
			},
			expectedLocal: map[string]string{
				"stack": "prod",
			},
			expectedPositional: []string{"component-name"},
		},
		{
			name:        "multiple persistent flags",
			parentFlags: []string{"logs-level", "config-path", "no-color"},
			childFlags:  []string{"dry-run"},
			args:        []string{"--logs-level=Debug", "--config-path=/tmp", "--no-color", "--dry-run"},
			expectedPersist: map[string]string{
				"logs-level":  "Debug",
				"config-path": "/tmp",
			},
			expectedLocal: map[string]string{
				"dry-run": "true",
			},
			expectedPositional: []string{},
		},
		{
			name:        "persistent flag with equals syntax",
			parentFlags: []string{"logs-level"},
			childFlags:  []string{"component"},
			args:        []string{"--logs-level=Debug", "my-component"},
			expectedPersist: map[string]string{
				"logs-level": "Debug",
			},
			expectedPositional: []string{"my-component"},
		},
		{
			name:        "persistent flag with space syntax",
			parentFlags: []string{"logs-level"},
			childFlags:  []string{"stack"},
			args:        []string{"--logs-level", "Debug", "--stack", "dev", "component"},
			expectedPersist: map[string]string{
				"logs-level": "Debug",
			},
			expectedLocal: map[string]string{
				"stack": "dev",
			},
			expectedPositional: []string{"component"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create parent command with persistent flags (simulates RootCmd).
			parentCmd := &cobra.Command{
				Use: "atmos",
			}

			// Register persistent flags on parent.
			for _, flagName := range tt.parentFlags {
				switch flagName {
				case "logs-level":
					parentCmd.PersistentFlags().String("logs-level", "", "Log level")
				case "config-path":
					parentCmd.PersistentFlags().String("config-path", "", "Config path")
				case "no-color":
					parentCmd.PersistentFlags().Bool("no-color", false, "Disable color")
				}
			}

			// Create child command with local flags and DisableFlagParsing=true.
			// Build child parser with only LOCAL flags (not persistent ones).
			childOpts := []Option{}
			for _, flagName := range tt.childFlags {
				switch flagName {
				case "component":
					childOpts = append(childOpts, func(cfg *parserConfig) {
						cfg.registry.Register(&StringFlag{Name: "component", Shorthand: "c"})
					})
				case "stack":
					childOpts = append(childOpts, func(cfg *parserConfig) {
						cfg.registry.Register(&StringFlag{Name: "stack", Shorthand: "s"})
					})
				case "dry-run":
					childOpts = append(childOpts, func(cfg *parserConfig) {
						cfg.registry.Register(&BoolFlag{Name: "dry-run"})
					})
				}
			}

			childParser := NewStandardFlagParser(childOpts...)

			childCmd := &cobra.Command{
				Use:  "subcommand",
				Args: cobra.ArbitraryArgs,
			}

			// Register flags and enable DisableFlagParsing.
			childParser.RegisterFlags(childCmd)
			parentCmd.AddCommand(childCmd)

			// Bind to Viper - bind both parent's persistent flags AND child's local flags.
			v := viper.New()

			// Manually bind parent's persistent flags to Viper (simulates global flag binding).
			for _, flagName := range tt.parentFlags {
				flag := parentCmd.PersistentFlags().Lookup(flagName)
				if flag != nil {
					v.BindPFlag(flagName, flag)
				}
			}

			err := childParser.BindToViper(v)
			require.NoError(t, err)

			// Parse the args.
			ctx := context.Background()
			result, err := childParser.Parse(ctx, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)

			// Verify persistent flags were parsed and bound to Viper.
			// Note: Persistent flags won't be in result.Flags because they're not in the child parser's registry.
			// In real code, persistent flags are accessed via Viper which was bound by the parent parser.
			for key, expected := range tt.expectedPersist {
				// Check that the flag was parsed (it's in Viper).
				actual := v.GetString(key)
				assert.Equal(t, expected, actual, "persistent flag %s has wrong value in Viper", key)
			}

			// Verify local flags were parsed.
			for key, expected := range tt.expectedLocal {
				actual, ok := result.Flags[key]
				assert.True(t, ok, "local flag %s not found", key)
				if expected == "true" {
					assert.True(t, actual.(bool), "local flag %s should be true", key)
				} else {
					assert.Equal(t, expected, actual, "local flag %s has wrong value", key)
				}
			}

			// Verify positional args.
			assert.Equal(t, tt.expectedPositional, result.PositionalArgs)
		})
	}
}

// TestStandardFlagParser_DisableFlagParsing_WithoutInheritedFlags tests that
// when DisableFlagParsing is enabled but the command has no parent with persistent flags,
// parsing still works correctly.
func TestStandardFlagParser_DisableFlagParsing_WithoutInheritedFlags(t *testing.T) {
	parser := NewStandardFlagParser(
		WithStringFlag("component", "c", "", "Component name"),
		WithStringFlag("stack", "s", "", "Stack name"),
	)

	cmd := &cobra.Command{
		Use:  "test",
		Args: cobra.ArbitraryArgs,
	}

	parser.RegisterFlags(cmd)
	assert.True(t, cmd.DisableFlagParsing, "DisableFlagParsing should be true")

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()
	result, err := parser.Parse(ctx, []string{"--component=vpc", "--stack=prod", "positional"})

	require.NoError(t, err)
	assert.Equal(t, "vpc", result.Flags["component"])
	assert.Equal(t, "prod", result.Flags["stack"])
	assert.Equal(t, []string{"positional"}, result.PositionalArgs)
}

// TestStandardFlagParser_RegisterPersistentFlags tests that RegisterPersistentFlags
// does NOT set DisableFlagParsing, allowing Cobra's normal flag parsing to work.
func TestStandardFlagParser_RegisterPersistentFlags(t *testing.T) {
	parser := NewStandardFlagParser(
		WithStringFlag("logs-level", "", "info", "Log level"),
		WithStringFlag("config-path", "", "", "Config path"),
	)

	cmd := &cobra.Command{
		Use: "root",
	}

	parser.RegisterPersistentFlags(cmd)

	// DisableFlagParsing should NOT be set by RegisterPersistentFlags.
	assert.False(t, cmd.DisableFlagParsing, "DisableFlagParsing should be false after RegisterPersistentFlags")

	// Verify persistent flags were registered.
	logsFlag := cmd.PersistentFlags().Lookup("logs-level")
	assert.NotNil(t, logsFlag)
	assert.Equal(t, "info", logsFlag.DefValue)

	configFlag := cmd.PersistentFlags().Lookup("config-path")
	assert.NotNil(t, configFlag)
}

// TestStandardFlagParser_CombinedFlagSet_ErrorHandling tests that errors
// from the combined FlagSet are properly propagated.
func TestStandardFlagParser_CombinedFlagSet_ErrorHandling(t *testing.T) {
	parser := NewStandardFlagParser(func(cfg *parserConfig) {
		cfg.registry.Register(&StringFlag{
			Name:        "format",
			Shorthand:   "f",
			Default:     "yaml",
			Description: "Output format",
			ValidValues: []string{"json", "yaml"},
		})
	})

	cmd := &cobra.Command{
		Use:  "test",
		Args: cobra.ArbitraryArgs,
	}

	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()

	// Test with invalid format value.
	result, err := parser.Parse(ctx, []string{"--format=invalid"})

	// Should return validation error.
	require.Error(t, err)
	assert.Nil(t, result)
	// Validation errors should wrap ErrInvalidFlagValue.
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	// Also check the error message contains helpful context.
	assert.Contains(t, err.Error(), "invalid value")
}

// TestStandardFlagParser_ArbitraryArgs_WithValidation tests the pattern we use
// for commands: Args: cobra.ArbitraryArgs with post-Parse validation.
func TestStandardFlagParser_ArbitraryArgs_WithValidation(t *testing.T) {
	parser := NewStandardFlagParser(
		WithStringFlag("stack", "s", "", "Stack name"),
	)

	cmd := &cobra.Command{
		Use:  "describe",
		Args: cobra.ArbitraryArgs, // Allow any args during Cobra phase.
		RunE: func(c *cobra.Command, args []string) error {
			// Parse flags first.
			opts, err := parser.Parse(context.Background(), args)
			if err != nil {
				return err
			}

			// Validate positional args AFTER parsing.
			positionalArgs := opts.PositionalArgs
			if len(positionalArgs) != 1 {
				return assert.AnError // Would be a proper error in real code.
			}

			return nil
		},
	}

	parser.RegisterFlags(cmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	// Test with correct number of args.
	err = cmd.RunE(cmd, []string{"component-1", "--stack=prod"})
	assert.NoError(t, err)

	// Test with incorrect number of args (too many).
	err = cmd.RunE(cmd, []string{"component-1", "component-2", "--stack=prod"})
	assert.Error(t, err)

	// Test with incorrect number of args (too few).
	err = cmd.RunE(cmd, []string{"--stack=prod"})
	assert.Error(t, err)
}

// TestStandardFlagParser_ShortFlags tests that short flag variants work correctly
// with the combined FlagSet for LOCAL flags.
func TestStandardFlagParser_ShortFlags(t *testing.T) {
	// Create child with local flags.
	parser := NewStandardFlagParser(
		WithStringFlag("stack", "s", "", "Stack name"),
		WithStringFlag("component", "c", "", "Component name"),
		WithStringFlag("format", "f", "yaml", "Output format"),
	)

	childCmd := &cobra.Command{
		Use:  "terraform",
		Args: cobra.ArbitraryArgs,
	}

	parser.RegisterFlags(childCmd)

	v := viper.New()
	err := parser.BindToViper(v)
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name: "long flags",
			args: []string{"--stack=prod", "--component=vpc", "--format=json"},
			expected: map[string]string{
				"stack":     "prod",
				"component": "vpc",
				"format":    "json",
			},
		},
		{
			name: "short flags",
			args: []string{"-s", "prod", "-c", "vpc", "-f", "json"},
			expected: map[string]string{
				"stack":     "prod",
				"component": "vpc",
				"format":    "json",
			},
		},
		{
			name: "mixed long and short flags",
			args: []string{"--stack=prod", "-c", "vpc", "-f", "json"},
			expected: map[string]string{
				"stack":     "prod",
				"component": "vpc",
				"format":    "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.Parse(ctx, tt.args)

			require.NoError(t, err)
			for key, expected := range tt.expected {
				actual, ok := result.Flags[key]
				assert.True(t, ok, "flag %s not found", key)
				assert.Equal(t, expected, actual, "flag %s has wrong value", key)
			}
		})
	}
}
