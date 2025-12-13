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

	t.Run("sets validator without prompts when no prompts configured", func(t *testing.T) {
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

		// Args should be set (the builder generates one).
		assert.NotNil(t, cmd.Args, "should set Args validator even without prompts")
	})
}

// TestStandardFlagParser_ValidateSingleFlag tests single flag validation.
func TestStandardFlagParser_ValidateSingleFlag(t *testing.T) {
	t.Run("returns nil for valid value", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml", "table"),
		)

		flags := map[string]interface{}{"format": "json"}

		err := parser.validateSingleFlag("format", []string{"json", "yaml", "table"}, flags, nil)
		assert.NoError(t, err, "should return nil for valid value")
	})

	t.Run("returns error for invalid value", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml", "table"),
		)

		flags := map[string]interface{}{"format": "xml"}

		err := parser.validateSingleFlag("format", []string{"json", "yaml", "table"}, flags, nil)
		assert.Error(t, err, "should return error for invalid value")
		assert.Contains(t, err.Error(), "xml", "error should mention invalid value")
		assert.Contains(t, err.Error(), "format", "error should mention flag name")
	})

	t.Run("returns nil for empty value", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "", "Output format"),
			WithValidValues("format", "json", "yaml", "table"),
		)

		flags := map[string]interface{}{"format": ""}

		err := parser.validateSingleFlag("format", []string{"json", "yaml", "table"}, flags, nil)
		assert.NoError(t, err, "should return nil for empty value")
	})

	t.Run("returns nil when flag not in result", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithStringFlag("format", "f", "json", "Output format"),
			WithValidValues("format", "json", "yaml", "table"),
		)

		flags := map[string]interface{}{} // format not present.

		err := parser.validateSingleFlag("format", []string{"json", "yaml", "table"}, flags, nil)
		assert.NoError(t, err, "should return nil when flag not in result")
	})
}
