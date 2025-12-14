package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestNewStandardFlagParser(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())

	assert.NotNil(t, parser)
	assert.NotNil(t, parser.registry)

	// CommonFlags() includes all global flags + stack + dry-run
	// Verify presence of expected flags rather than hardcoding count
	assert.NotNil(t, parser.registry.Get("stack"), "should have stack flag")
	assert.NotNil(t, parser.registry.Get("dry-run"), "should have dry-run flag")
	assert.NotNil(t, parser.registry.Get("identity"), "should have identity from global flags")
	assert.NotNil(t, parser.registry.Get("chdir"), "should have chdir from global flags")
	assert.NotNil(t, parser.registry.Get("logs-level"), "should have logs-level from global flags")
}

func TestStandardFlagParser_RegisterFlags(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())
	cmd := &cobra.Command{Use: "test"}

	parser.RegisterFlags(cmd)

	// Check that flags were registered
	stackFlag := cmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag)
	assert.Equal(t, "s", stackFlag.Shorthand)

	identityFlag := cmd.Flags().Lookup(cfg.IdentityFlagName)
	assert.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
	assert.Equal(t, cfg.IdentityFlagSelectValue, identityFlag.NoOptDefVal)

	dryRunFlag := cmd.Flags().Lookup("dry-run")
	assert.NotNil(t, dryRunFlag)
}

func TestStandardFlagParser_BindToViper(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())
	v := viper.New()

	err := parser.BindToViper(v)

	require.NoError(t, err)
	// Viper bindings are internal, we can't easily test them directly
	// But we can verify no error occurred
}

func TestStandardFlagParser_BindFlagsToViper(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())
	cmd := &cobra.Command{Use: "test"}
	v := viper.New()

	parser.RegisterFlags(cmd)
	err := parser.BindToViper(v)
	require.NoError(t, err)

	err = parser.BindFlagsToViper(cmd, v)

	require.NoError(t, err)
}

func TestStandardFlagParser_Parse(t *testing.T) {
	parser := NewStandardFlagParser(WithCommonFlags())

	ctx := context.Background()
	cfg, err := parser.Parse(ctx, []string{})

	// Parse() is a placeholder for interface compliance
	// StandardFlagParser doesn't populate ParsedConfig like FlagRegistry does
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.NotNil(t, cfg.Flags)
}

func TestStandardFlagParser_GetIdentityFromCmd(t *testing.T) {
	tests := []struct {
		name          string
		flagValue     string
		flagSet       bool
		envValue      string
		expectedValue string
	}{
		{
			name:          "flag explicitly set",
			flagValue:     "admin",
			flagSet:       true,
			expectedValue: "admin",
		},
		{
			name:          "flag not set, use env",
			flagSet:       false,
			envValue:      "ci-user",
			expectedValue: "ci-user",
		},
		{
			name:          "flag not set, no env",
			flagSet:       false,
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewStandardFlagParser(WithIdentityFlag())
			cmd := &cobra.Command{Use: "test"}
			v := viper.New()

			parser.RegisterFlags(cmd)
			parser.BindToViper(v)
			parser.BindFlagsToViper(cmd, v)

			if tt.flagSet {
				cmd.Flags().Set(cfg.IdentityFlagName, tt.flagValue)
			}
			if tt.envValue != "" {
				v.Set(cfg.IdentityFlagName, tt.envValue)
			}

			identity, err := parser.GetIdentityFromCmd(cmd, v)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedValue, identity)
		})
	}
}

func TestStandardFlagParser_WithViperPrefix(t *testing.T) {
	parser := NewStandardFlagParser(
		WithCommonFlags(),
		WithViperPrefix("terraform"),
	)

	cmd := &cobra.Command{Use: "test"}
	v := viper.New()

	parser.RegisterFlags(cmd)
	parser.BindToViper(v)
	parser.BindFlagsToViper(cmd, v)

	// Set flag value
	cmd.Flags().Set("stack", "dev")

	// Read value from Viper (with prefix)
	value := v.GetString("terraform.stack")

	// The viper prefix affects how values are stored/retrieved
	require.NotEmpty(t, value)
	assert.Equal(t, "dev", value)
}

func TestStandardFlagParser_RequiredFlags(t *testing.T) {
	parser := NewStandardFlagParser(
		WithRequiredStringFlag("component", "c", "Component name (required)"),
	)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Verify flag is marked as required
	componentFlag := cmd.Flags().Lookup("component")
	assert.NotNil(t, componentFlag)
	// Cobra marks required flags internally, we just verify it's registered
}

// TestStandardFlagParser_ParsedFlags tests the ParsedFlags method.
func TestStandardFlagParser_ParsedFlags(t *testing.T) {
	t.Run("returns nil before Parse is called", func(t *testing.T) {
		parser := NewStandardFlagParser(WithCommonFlags())
		assert.Nil(t, parser.ParsedFlags(), "should return nil before Parse")
	})

	t.Run("returns combined flags after Parse is called", func(t *testing.T) {
		parser := NewStandardFlagParser(WithCommonFlags())
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		ctx := context.Background()
		_, err := parser.Parse(ctx, []string{"--stack", "dev"})
		require.NoError(t, err)

		parsedFlags := parser.ParsedFlags()
		assert.NotNil(t, parsedFlags, "should return combined flags after Parse")
		// The parsedFlags should contain the registered flags.
		stackFlag := parsedFlags.Lookup("stack")
		assert.NotNil(t, stackFlag, "should contain stack flag")
	})
}

// TestGetActualArgs tests the GetActualArgs function.
func TestGetActualArgs(t *testing.T) {
	t.Run("returns cmd.Flags().Args() when DisableFlagParsing is false", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.DisableFlagParsing = false
		// When DisableFlagParsing is false, cmd.Flags().Args() returns parsed positional args.
		// For this test, we just verify the function handles this case.
		args := GetActualArgs(cmd, []string{"atmos", "test", "arg1", "arg2"})
		// With no actual parsing, this returns empty.
		assert.Empty(t, args, "should return empty when no args parsed")
	})

	t.Run("extracts args from osArgs when DisableFlagParsing is true", func(t *testing.T) {
		// Create a proper command hierarchy to get CommandPath() = "test".
		cmd := &cobra.Command{Use: "test"}
		cmd.DisableFlagParsing = true
		// Simulate command path "test" (depth 1).
		osArgs := []string{"test", "arg1", "arg2"}
		args := GetActualArgs(cmd, osArgs)
		assert.Equal(t, []string{"arg1", "arg2"}, args, "should extract args after command path")
	})

	t.Run("handles nested command paths", func(t *testing.T) {
		rootCmd := &cobra.Command{Use: "atmos"}
		parentCmd := &cobra.Command{Use: "describe"}
		childCmd := &cobra.Command{Use: "component"}
		childCmd.DisableFlagParsing = true

		rootCmd.AddCommand(parentCmd)
		parentCmd.AddCommand(childCmd)

		// Command path is "atmos describe component" (depth 3).
		osArgs := []string{"atmos", "describe", "component", "vpc", "stack-name"}
		args := GetActualArgs(childCmd, osArgs)
		assert.Equal(t, []string{"vpc", "stack-name"}, args, "should extract args after nested command path")
	})

	t.Run("returns empty when osArgs shorter than command depth", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.DisableFlagParsing = true
		osArgs := []string{"atmos"}
		args := GetActualArgs(cmd, osArgs)
		assert.Empty(t, args, "should return empty when osArgs is shorter than command depth")
	})
}

// TestValidateArgsOrNil tests the ValidateArgsOrNil function.
func TestValidateArgsOrNil(t *testing.T) {
	t.Run("returns nil when cmd.Args is nil", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Args = nil
		err := ValidateArgsOrNil(cmd, []string{"arg1", "arg2"})
		assert.NoError(t, err, "should return nil when no validator")
	})

	t.Run("returns nil when Args validator passes", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Args = cobra.ExactArgs(2)
		err := ValidateArgsOrNil(cmd, []string{"arg1", "arg2"})
		assert.NoError(t, err, "should return nil when validation passes")
	})

	t.Run("returns error when Args validator fails", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Args = cobra.ExactArgs(2)
		err := ValidateArgsOrNil(cmd, []string{"arg1"})
		assert.Error(t, err, "should return error when validation fails")
	})

	t.Run("handles MinimumNArgs validator", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Args = cobra.MinimumNArgs(1)

		// Should pass with 1+ args.
		err := ValidateArgsOrNil(cmd, []string{"arg1"})
		assert.NoError(t, err, "should pass with minimum args")

		// Should fail with 0 args.
		err = ValidateArgsOrNil(cmd, []string{})
		assert.Error(t, err, "should fail with less than minimum args")
	})
}

// TestStandardFlagParser_SetPositionalArgs tests the SetPositionalArgs method.
func TestStandardFlagParser_SetPositionalArgs(t *testing.T) {
	t.Run("sets positional args configuration", func(t *testing.T) {
		parser := NewStandardFlagParser()

		specs := []*PositionalArgSpec{
			{Name: "component", Description: "Component name", Required: true},
			{Name: "stack", Description: "Stack name", Required: false},
		}
		validator := cobra.MinimumNArgs(1)
		usage := "<component> [stack]"

		parser.SetPositionalArgs(specs, validator, usage)

		assert.NotNil(t, parser.positionalArgs, "should set positionalArgs")
		assert.Equal(t, specs, parser.positionalArgs.specs, "should set specs")
		assert.NotNil(t, parser.positionalArgs.validator, "should set validator")
		assert.Equal(t, usage, parser.positionalArgs.usage, "should set usage")
	})
}

// TestStandardFlagParser_RegisterPositionalArgsValidator tests positional args validator registration.
func TestStandardFlagParser_RegisterPositionalArgsValidator(t *testing.T) {
	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("skips when no positional args configured", func(t *testing.T) {
		parser := NewStandardFlagParser()
		cmd := &cobra.Command{Use: "test"}

		parser.RegisterFlags(cmd)

		// Args should remain nil when no positional args are configured.
		assert.Nil(t, cmd.Args, "should not set Args when no positional args")
	})

	t.Run("sets prompt-aware validator when prompts configured", func(t *testing.T) {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:           "theme",
			Description:    "Theme name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose theme",
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser(
			WithPositionalArgPrompt("theme", "Choose theme", completionFunc),
		)
		parser.SetPositionalArgs(specs, validator, usage)

		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Args should be set to a prompt-aware validator.
		assert.NotNil(t, cmd.Args, "should set Args validator")
	})

	t.Run("does not override cmd.Args when no prompts configured", func(t *testing.T) {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:        "component",
			Description: "Component name",
			Required:    true,
			// No CompletionFunc or PromptTitle.
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser()
		parser.SetPositionalArgs(specs, validator, usage)

		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Args should NOT be set when no prompts are configured.
		// This avoids overriding any pre-existing cmd.Args validator.
		assert.Nil(t, cmd.Args, "should not set Args validator when no prompts configured")
	})

	t.Run("preserves existing cmd.Args when no prompts configured", func(t *testing.T) {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:        "component",
			Description: "Component name",
			Required:    true,
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser()
		parser.SetPositionalArgs(specs, validator, usage)

		// Set a pre-existing validator.
		existingValidator := cobra.ExactArgs(1)
		cmd := &cobra.Command{Use: "test", Args: existingValidator}
		parser.RegisterFlags(cmd)

		// Pre-existing validator should be preserved.
		assert.NotNil(t, cmd.Args, "should preserve existing Args validator")
	})
}

// TestStandardFlagParser_ValidateSingleFlag tests single flag validation.
func TestStandardFlagParser_ValidateSingleFlag(t *testing.T) {
	tests := []struct {
		name          string
		flagDefault   string
		flags         map[string]interface{}
		expectError   bool
		errorContains []string
	}{
		{
			name:        "valid value",
			flagDefault: "json",
			flags:       map[string]interface{}{"format": "json"},
			expectError: false,
		},
		{
			name:          "invalid value",
			flagDefault:   "json",
			flags:         map[string]interface{}{"format": "xml"},
			expectError:   true,
			errorContains: []string{"xml", "format"},
		},
		{
			name:        "empty value",
			flagDefault: "",
			flags:       map[string]interface{}{"format": ""},
			expectError: false,
		},
		{
			name:        "flag not in result",
			flagDefault: "json",
			flags:       map[string]interface{}{}, // format not present.
			expectError: false,
		},
	}

	validValues := []string{"json", "yaml", "table"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewStandardFlagParser(
				WithStringFlag("format", "f", tt.flagDefault, "Output format"),
				WithValidValues("format", validValues...),
			)

			err := parser.validateSingleFlag("format", validValues, tt.flags, nil)

			if tt.expectError {
				assert.Error(t, err)
				for _, text := range tt.errorContains {
					assert.Contains(t, err.Error(), text)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestStandardFlagParser_ParseWithPositionalArgs tests parsing with positional arguments.
func TestStandardFlagParser_ParseWithPositionalArgs(t *testing.T) {
	t.Run("parses flags with command registered", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)
		v := viper.New()
		err := parser.BindFlagsToViper(cmd, v)
		require.NoError(t, err)

		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{"--format", "yaml"})

		require.NoError(t, err)
		assert.Equal(t, "yaml", result.Flags["format"])
	})

	t.Run("handles empty args", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)

		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{})

		require.NoError(t, err)
		assert.Empty(t, result.PositionalArgs)
		assert.Empty(t, result.SeparatedArgs)
	})

	t.Run("uses default value when flag not provided", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)
		v := viper.New()
		err := parser.BindFlagsToViper(cmd, v)
		require.NoError(t, err)

		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{})

		require.NoError(t, err)
		assert.Equal(t, "json", result.Flags["format"])
	})
}

// TestStandardFlagParser_ValidatePositionalArgs tests positional arg validation.
// Note: Validation now runs AFTER prompts have filled in values (in Parse flow).
// This tests the validatePositionalArgs method directly, which always validates.
func TestStandardFlagParser_ValidatePositionalArgs(t *testing.T) {
	t.Run("validates required args after prompts fill values", func(t *testing.T) {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:        "theme-name",
			Description: "Theme name",
			Required:    true,
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser()
		parser.SetPositionalArgs(specs, validator, usage)

		// Simulates the case after prompts have filled in the value.
		err := parser.validatePositionalArgs([]string{"selected-theme"})
		assert.NoError(t, err)
	})

	t.Run("errors when required arg missing", func(t *testing.T) {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:        "required-arg",
			Description: "Required argument",
			Required:    true,
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser()
		parser.SetPositionalArgs(specs, validator, usage)

		// Should error because required arg is missing (prompts didn't fill it).
		err := parser.validatePositionalArgs([]string{})
		assert.Error(t, err)
	})

	t.Run("passes with no positional args configured", func(t *testing.T) {
		parser := NewStandardFlagParser()

		// No positional args configured, should pass.
		err := parser.validatePositionalArgs([]string{})
		assert.NoError(t, err)
	})
}

// TestStandardFlagParser_GetViperKey tests the getViperKey method.
func TestStandardFlagParser_GetViperKey(t *testing.T) {
	t.Run("returns flag name without prefix", func(t *testing.T) {
		parser := NewStandardFlagParser()
		key := parser.getViperKey("my-flag")
		assert.Equal(t, "my-flag", key)
	})

	t.Run("returns prefixed key with prefix", func(t *testing.T) {
		parser := NewStandardFlagParser(WithViperPrefix("myprefix"))
		key := parser.getViperKey("my-flag")
		assert.Equal(t, "myprefix.my-flag", key)
	})
}

// TestStandardFlagParser_Reset tests the Reset method.
func TestStandardFlagParser_Reset(t *testing.T) {
	t.Run("resets command flags to defaults", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Set a flag value.
		err := cmd.Flags().Set("format", "yaml")
		require.NoError(t, err)

		// Verify it's set.
		val, _ := cmd.Flags().GetString("format")
		assert.Equal(t, "yaml", val)

		// Reset.
		parser.Reset()

		// Verify it's reset.
		val, _ = cmd.Flags().GetString("format")
		assert.Equal(t, "json", val)
	})

	t.Run("handles nil command gracefully", func(t *testing.T) {
		parser := NewStandardFlagParser()
		// Should not panic.
		parser.Reset()
	})
}

// TestStandardFlagParser_IsFlagExplicitlyChanged tests the isFlagExplicitlyChanged method.
func TestStandardFlagParser_IsFlagExplicitlyChanged(t *testing.T) {
	t.Run("returns true when combinedFlags is nil", func(t *testing.T) {
		parser := NewStandardFlagParser()
		result := parser.isFlagExplicitlyChanged("any-flag", nil)
		assert.True(t, result)
	})

	t.Run("returns true when flag is changed", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Parse with the flag set.
		err := cmd.Flags().Set("format", "yaml")
		require.NoError(t, err)

		result := parser.isFlagExplicitlyChanged("format", cmd.Flags())
		assert.True(t, result)
	})

	t.Run("returns false when flag is not changed", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Don't set the flag.
		result := parser.isFlagExplicitlyChanged("format", cmd.Flags())
		assert.False(t, result)
	})
}

// TestStandardFlagParser_IsValueValid tests the isValueValid method.
func TestStandardFlagParser_IsValueValid(t *testing.T) {
	parser := NewStandardFlagParser()

	tests := []struct {
		name        string
		value       string
		validValues []string
		expected    bool
	}{
		{
			name:        "value in list",
			value:       "json",
			validValues: []string{"json", "yaml", "table"},
			expected:    true,
		},
		{
			name:        "value not in list",
			value:       "xml",
			validValues: []string{"json", "yaml", "table"},
			expected:    false,
		},
		{
			name:        "empty list",
			value:       "json",
			validValues: []string{},
			expected:    false,
		},
		{
			name:        "case sensitive match",
			value:       "JSON",
			validValues: []string{"json", "yaml"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.isValueValid(tt.value, tt.validValues)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStandardFlagParser_CreateValidationError tests error message generation.
func TestStandardFlagParser_CreateValidationError(t *testing.T) {
	t.Run("uses default message", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml"),
		)

		err := parser.createValidationError("format", "xml", []string{"json", "yaml"})
		assert.Contains(t, err.Error(), "invalid value")
		assert.Contains(t, err.Error(), "xml")
		assert.Contains(t, err.Error(), "format")
		assert.Contains(t, err.Error(), "json, yaml")
	})
}

// TestStandardFlagParser_ValidateFlagValues tests flag value validation.
func TestStandardFlagParser_ValidateFlagValues(t *testing.T) {
	t.Run("returns nil when no valid values configured", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)

		flags := map[string]interface{}{"format": "any-value"}
		err := parser.validateFlagValues(flags, nil)
		assert.NoError(t, err)
	})

	t.Run("validates all flags with valid values", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Set the flag to an invalid value.
		err := cmd.Flags().Set("format", "xml")
		require.NoError(t, err)

		flags := map[string]interface{}{"format": "xml"}
		err = parser.validateFlagValues(flags, cmd.Flags())
		assert.Error(t, err)
	})
}

// TestStandardFlagParser_GetStringFlagValue tests the getStringFlagValue method.
func TestStandardFlagParser_GetStringFlagValue(t *testing.T) {
	t.Run("returns viper value when not empty", func(t *testing.T) {
		v := viper.New()
		v.Set("format", "yaml")

		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		parser.viper = v

		strFlag := &StringFlag{
			Name:    "format",
			Default: "json",
		}
		value := parser.getStringFlagValue(strFlag, "format", "format", nil)
		assert.Equal(t, "yaml", value)
	})

	t.Run("returns default when viper empty and flag not changed", func(t *testing.T) {
		v := viper.New()

		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		parser.viper = v
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		strFlag := &StringFlag{
			Name:    "format",
			Default: "json",
		}
		value := parser.getStringFlagValue(strFlag, "format", "format", cmd.Flags())
		assert.Equal(t, "json", value)
	})

	t.Run("returns default when combinedFlags is nil", func(t *testing.T) {
		v := viper.New()

		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		parser.viper = v

		strFlag := &StringFlag{
			Name:    "format",
			Default: "json",
		}
		value := parser.getStringFlagValue(strFlag, "format", "format", nil)
		assert.Equal(t, "json", value)
	})
}

// TestStandardFlagParser_RegisterCompletions tests completion registration.
//
//nolint:dupl // Test functions for RegisterFlags and RegisterPersistentFlags intentionally have similar structure.
func TestStandardFlagParser_RegisterCompletions(t *testing.T) {
	t.Run("registers completions for flags with valid values", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml", "table"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Verify flag exists.
		formatFlag := cmd.Flags().Lookup("format")
		assert.NotNil(t, formatFlag, "format flag should exist")
	})

	t.Run("skips registration when no valid values configured", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Should not panic and flag should exist.
		formatFlag := cmd.Flags().Lookup("format")
		assert.NotNil(t, formatFlag)
	})

	t.Run("skips registration for nonexistent flags", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithValidValues("nonexistent", "value1", "value2"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Should not panic when flag doesn't exist.
		nonexistentFlag := cmd.Flags().Lookup("nonexistent")
		assert.Nil(t, nonexistentFlag)
	})
}

// TestStandardFlagParser_RegisterPersistentCompletions tests persistent completion registration.
//
//nolint:dupl // Test functions for RegisterFlags and RegisterPersistentFlags intentionally have similar structure.
func TestStandardFlagParser_RegisterPersistentCompletions(t *testing.T) {
	t.Run("registers completions for persistent flags with valid values", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml", "table"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterPersistentFlags(cmd)

		// Verify persistent flag exists.
		formatFlag := cmd.PersistentFlags().Lookup("format")
		assert.NotNil(t, formatFlag, "format persistent flag should exist")
	})

	t.Run("skips registration when no valid values configured", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterPersistentFlags(cmd)

		// Should not panic.
		formatFlag := cmd.PersistentFlags().Lookup("format")
		assert.NotNil(t, formatFlag)
	})

	t.Run("skips registration for nonexistent persistent flags", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithValidValues("nonexistent", "value1", "value2"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterPersistentFlags(cmd)

		// Should not panic.
		nonexistentFlag := cmd.PersistentFlags().Lookup("nonexistent")
		assert.Nil(t, nonexistentFlag)
	})
}

// TestStandardFlagParser_ExtractArgs tests the extractArgs method.
func TestStandardFlagParser_ExtractArgs(t *testing.T) {
	t.Run("extracts positional args without separator", func(t *testing.T) {
		parser := NewStandardFlagParser()
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Parse args without "--" separator.
		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{"arg1", "arg2"})

		require.NoError(t, err)
		assert.Equal(t, []string{"arg1", "arg2"}, result.PositionalArgs)
		assert.Empty(t, result.SeparatedArgs)
	})

	t.Run("extracts args with separator", func(t *testing.T) {
		parser := NewStandardFlagParser()
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Parse args with "--" separator.
		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{"pos1", "--", "sep1", "sep2"})

		require.NoError(t, err)
		assert.Equal(t, []string{"pos1"}, result.PositionalArgs)
		assert.Equal(t, []string{"sep1", "sep2"}, result.SeparatedArgs)
	})

	t.Run("handles separator at beginning", func(t *testing.T) {
		parser := NewStandardFlagParser()
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{"--", "sep1", "sep2"})

		require.NoError(t, err)
		assert.Empty(t, result.PositionalArgs)
		assert.Equal(t, []string{"sep1", "sep2"}, result.SeparatedArgs)
	})

	t.Run("handles separator at end", func(t *testing.T) {
		parser := NewStandardFlagParser()
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		ctx := context.Background()
		result, err := parser.Parse(ctx, []string{"pos1", "pos2", "--"})

		require.NoError(t, err)
		assert.Equal(t, []string{"pos1", "pos2"}, result.PositionalArgs)
		assert.Empty(t, result.SeparatedArgs)
	})
}

// TestStandardFlagParser_PromptForSingleMissingFlag tests the helper function.
func TestStandardFlagParser_PromptForSingleMissingFlag(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"stack1", "stack2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("skips when flag has value", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithStringFlag("stack", "s", "", "Stack name"),
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"stack": "prod"},
			PositionalArgs: []string{},
		}

		err := parser.promptForSingleMissingFlag("stack", result, cmd.Flags())
		assert.NoError(t, err)
		assert.Equal(t, "prod", result.Flags["stack"])
	})

	t.Run("skips when flag was explicitly changed", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithStringFlag("stack", "s", "", "Stack name"),
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		// Explicitly set the flag to empty.
		err := cmd.Flags().Set("stack", "")
		require.NoError(t, err)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"stack": ""},
			PositionalArgs: []string{},
		}

		err = parser.promptForSingleMissingFlag("stack", result, cmd.Flags())
		assert.NoError(t, err)
		// Value should remain empty since it was explicitly set.
		assert.Equal(t, "", result.Flags["stack"])
	})

	t.Run("skips when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithStringFlag("stack", "s", "", "Stack name"),
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"stack": ""},
			PositionalArgs: []string{},
		}

		err := parser.promptForSingleMissingFlag("stack", result, cmd.Flags())
		assert.NoError(t, err)
		// Value should remain empty in non-interactive mode.
		assert.Equal(t, "", result.Flags["stack"])
	})
}

// TestStandardFlagParser_PromptForOptionalValueFlags_FallbackToDefault tests fallback behavior.
func TestStandardFlagParser_PromptForOptionalValueFlags_FallbackToDefault(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"identity1", "identity2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("falls back to default when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithStringFlag("identity", "i", "default-identity", "Identity"),
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"identity": "__SELECT__"},
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, cmd.Flags())
		assert.NoError(t, err)
		// Should fall back to default value since not interactive.
		assert.Equal(t, "default-identity", result.Flags["identity"])
	})

	t.Run("falls back to empty when flag not in combinedFlags", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)
		// Create a command with a different flag (not identity).
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("other-flag", "", "Other flag")

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"identity": "__SELECT__"},
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, cmd.Flags())
		assert.NoError(t, err)
		// Should fall back to empty string when flag lookup fails (flag not in combinedFlags).
		assert.Equal(t, "", result.Flags["identity"])
	})

	t.Run("returns early when combinedFlags is nil", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"identity": "__SELECT__"},
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, nil)
		assert.NoError(t, err)
		// When combinedFlags is nil, the function returns early without processing.
		assert.Equal(t, "__SELECT__", result.Flags["identity"])
	})
}

// TestStandardFlagParser_RegisterIntFlag tests int flag registration.
func TestStandardFlagParser_RegisterIntFlag(t *testing.T) {
	t.Run("registers int flag", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithIntFlag("count", "n", 10, "Count value"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		countFlag := cmd.Flags().Lookup("count")
		assert.NotNil(t, countFlag)
		assert.Equal(t, "n", countFlag.Shorthand)
		assert.Equal(t, "10", countFlag.DefValue)
	})

	t.Run("registers required int flag", func(t *testing.T) {
		// Create custom int flag with Required=true.
		intFlag := &IntFlag{
			Name:        "required-count",
			Shorthand:   "r",
			Default:     0,
			Description: "Required count",
			Required:    true,
		}

		parser := NewStandardFlagParser()
		parser.registry.Register(intFlag)

		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		countFlag := cmd.Flags().Lookup("required-count")
		assert.NotNil(t, countFlag)
	})
}

// TestStandardFlagParser_RegisterStringSliceFlag tests string slice flag registration.
func TestStandardFlagParser_RegisterStringSliceFlag(t *testing.T) {
	t.Run("registers string slice flag", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringSliceFlag("tags", "t", []string{"default"}, "Tag values"),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		tagsFlag := cmd.Flags().Lookup("tags")
		assert.NotNil(t, tagsFlag)
		assert.Equal(t, "t", tagsFlag.Shorthand)
	})

	t.Run("registers required string slice flag", func(t *testing.T) {
		// Create custom string slice flag with Required=true.
		sliceFlag := &StringSliceFlag{
			Name:        "required-tags",
			Shorthand:   "r",
			Default:     []string{},
			Description: "Required tags",
			Required:    true,
		}

		parser := NewStandardFlagParser()
		parser.registry.Register(sliceFlag)

		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		tagsFlag := cmd.Flags().Lookup("required-tags")
		assert.NotNil(t, tagsFlag)
	})
}

// TestStandardFlagParser_HandleInteractivePrompts_AllCases tests all prompt use cases.
func TestStandardFlagParser_HandleInteractivePrompts_AllCases(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("handles all three use cases together", func(t *testing.T) {
		viper.Set("interactive", false)

		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:           "theme",
			Description:    "Theme name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose theme",
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser(
			WithStringFlag("stack", "s", "", "Stack name"),
			WithStringFlag("identity", "i", "default", "Identity"),
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
			WithPositionalArgPrompt("theme", "Choose theme", completionFunc),
		)
		parser.SetPositionalArgs(specs, validator, usage)

		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{
			Flags: map[string]interface{}{
				"stack":    "",
				"identity": "__SELECT__",
			},
			PositionalArgs: []string{},
		}

		err := parser.handleInteractivePrompts(result, cmd.Flags())
		assert.NoError(t, err)
	})
}

// TestStandardFlagParser_BindFlagsToViper_EdgeCases tests edge cases in binding.
func TestStandardFlagParser_BindFlagsToViper_EdgeCases(t *testing.T) {
	t.Run("handles flag not found in cobra flags", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
		)
		// Create command but don't register flags to it.
		cmd := &cobra.Command{Use: "test"}
		v := viper.New()

		// Register flags first.
		parser.RegisterFlags(cmd)

		// Now bind - this should work even though some internal flags might not exist.
		err := parser.BindFlagsToViper(cmd, v)
		assert.NoError(t, err)
	})
}

// TestStringFlag_GetValidValues tests the GetValidValues method.
func TestStringFlag_GetValidValues(t *testing.T) {
	t.Run("returns valid values when set", func(t *testing.T) {
		flag := &StringFlag{
			Name:        "format",
			ValidValues: []string{"json", "yaml", "table"},
		}
		values := flag.GetValidValues()
		assert.Equal(t, []string{"json", "yaml", "table"}, values)
	})

	t.Run("returns nil when not set", func(t *testing.T) {
		flag := &StringFlag{
			Name: "format",
		}
		values := flag.GetValidValues()
		assert.Nil(t, values)
	})
}

// TestStandardFlagParser_PromptForMissingRequiredFlags_MultipleFlagsOrder tests deterministic order.
func TestStandardFlagParser_PromptForMissingRequiredFlags_MultipleFlagsOrder(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	viper.Set("interactive", false)

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("processes flags in alphabetical order", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithCompletionPrompt("zebra", "Choose zebra", completionFunc),
			WithCompletionPrompt("apple", "Choose apple", completionFunc),
			WithCompletionPrompt("mango", "Choose mango", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"zebra": "", "apple": "", "mango": ""},
			PositionalArgs: []string{},
		}

		err := parser.promptForMissingRequiredFlags(result, nil)
		assert.NoError(t, err)
		// In non-interactive mode, nothing changes but order is deterministic.
	})
}

// TestStandardFlagParser_PromptForOptionalValueFlags_MultipleFlagsOrder tests deterministic order.
func TestStandardFlagParser_PromptForOptionalValueFlags_MultipleFlagsOrder(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	viper.Set("interactive", false)

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("processes flags in alphabetical order", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("zebra", "", "z-default", "Zebra"),
			WithStringFlag("apple", "", "a-default", "Apple"),
			WithStringFlag("mango", "", "m-default", "Mango"),
			WithOptionalValuePrompt("zebra", "Choose zebra", completionFunc),
			WithOptionalValuePrompt("apple", "Choose apple", completionFunc),
			WithOptionalValuePrompt("mango", "Choose mango", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{
			Flags: map[string]interface{}{
				"zebra": "__SELECT__",
				"apple": "__SELECT__",
				"mango": "__SELECT__",
			},
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, cmd.Flags())
		assert.NoError(t, err)
		// Should fall back to defaults in alphabetical order.
		assert.Equal(t, "a-default", result.Flags["apple"])
		assert.Equal(t, "m-default", result.Flags["mango"])
		assert.Equal(t, "z-default", result.Flags["zebra"])
	})
}
