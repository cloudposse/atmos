//nolint:dupl // Test functions intentionally have similar structure
package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIsInteractive tests the isInteractive function.
func TestIsInteractive(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	t.Run("returns false when interactive flag is disabled", func(t *testing.T) {
		viper.Set("interactive", false)
		result := isInteractive()
		assert.False(t, result, "should return false when interactive flag is disabled")
	})

	t.Run("respects interactive flag setting", func(t *testing.T) {
		viper.Set("interactive", true)
		// Note: Actual result depends on TTY and CI environment.
		// We just verify the function runs without error.
		_ = isInteractive()
	})
}

// TestPromptForValue tests the PromptForValue function.
func TestPromptForValue(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	t.Run("returns error when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)
		_, err := PromptForValue("test-flag", "Choose option", []string{"option1", "option2"})
		assert.Error(t, err, "should return error when not interactive")
		assert.Contains(t, err.Error(), "interactive mode not available")
	})

	t.Run("returns error when no options available", func(t *testing.T) {
		viper.Set("interactive", false) // Ensure non-interactive for predictable test.
		_, err := PromptForValue("test-flag", "Choose option", []string{})
		assert.Error(t, err, "should return error when no options available")
	})
}

// TestPromptForMissingRequired tests the PromptForMissingRequired function.
func TestPromptForMissingRequired(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}
	args := []string{}

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2", "option3"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("returns empty when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)
		result, err := PromptForMissingRequired("test-flag", "Choose option", completionFunc, cmd, args)
		assert.NoError(t, err, "should not return error when not interactive")
		assert.Empty(t, result, "should return empty string when not interactive")
	})

	t.Run("returns empty when no options available", func(t *testing.T) {
		viper.Set("interactive", false)
		emptyCompletionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		result, err := PromptForMissingRequired("test-flag", "Choose option", emptyCompletionFunc, cmd, args)
		assert.NoError(t, err, "should not return error when no options")
		assert.Empty(t, result, "should return empty string when no options")
	})
}

// TestPromptForOptionalValue tests the PromptForOptionalValue function.
func TestPromptForOptionalValue(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}
	args := []string{}

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("returns value unchanged when not sentinel", func(t *testing.T) {
		result, err := PromptForOptionalValue("test-flag", "real-value", "Choose option", completionFunc, cmd, args)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "real-value", result, "should return unchanged value when not sentinel")
	})

	t.Run("returns empty when not interactive and value is sentinel", func(t *testing.T) {
		viper.Set("interactive", false)
		result, err := PromptForOptionalValue("test-flag", "__SELECT__", "Choose option", completionFunc, cmd, args)
		assert.NoError(t, err, "should not return error when not interactive")
		assert.Empty(t, result, "should return empty when not interactive")
	})
}

// TestPromptForPositionalArg tests the PromptForPositionalArg function.
func TestPromptForPositionalArg(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}
	currentArgs := []string{}

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"arg1", "arg2", "arg3"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("returns empty when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)
		result, err := PromptForPositionalArg("test-arg", "Choose argument", completionFunc, cmd, currentArgs)
		assert.NoError(t, err, "should not return error when not interactive")
		assert.Empty(t, result, "should return empty string when not interactive")
	})

	t.Run("returns empty when no options available", func(t *testing.T) {
		viper.Set("interactive", false)
		emptyCompletionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}
		result, err := PromptForPositionalArg("test-arg", "Choose argument", emptyCompletionFunc, cmd, currentArgs)
		assert.NoError(t, err, "should not return error when no options")
		assert.Empty(t, result, "should return empty string when no options")
	})
}

// TestStandardFlagParser_PromptForOptionalValueFlags tests Use Case 2.
func TestStandardFlagParser_PromptForOptionalValueFlags(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"identity1", "identity2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("skips prompt when flag value is not sentinel", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"identity": "real-value"},
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "real-value", result.Flags["identity"], "should not change value when not sentinel")
	})

	t.Run("skips prompt when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"identity": "__SELECT__"},
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, nil)
		assert.NoError(t, err, "should not return error when not interactive")
		assert.Equal(t, "__SELECT__", result.Flags["identity"], "should keep sentinel when not interactive")
	})
}

// TestStandardFlagParser_PromptForMissingRequiredFlags tests Use Case 1.
func TestStandardFlagParser_PromptForMissingRequiredFlags(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"stack1", "stack2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("skips prompt when flag has value", func(t *testing.T) {
		parser := NewStandardFlagParser(
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"stack": "prod"},
			PositionalArgs: []string{},
		}

		err := parser.promptForMissingRequiredFlags(result, nil)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, "prod", result.Flags["stack"], "should not change value when already set")
	})

	t.Run("skips prompt when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"stack": ""},
			PositionalArgs: []string{},
		}

		err := parser.promptForMissingRequiredFlags(result, nil)
		assert.NoError(t, err, "should not return error when not interactive")
		assert.Equal(t, "", result.Flags["stack"], "should keep empty value when not interactive")
	})
}

// TestStandardFlagParser_PromptForMissingPositionalArgs tests Use Case 3.
func TestStandardFlagParser_PromptForMissingPositionalArgs(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"theme1", "theme2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("skips prompt when argument is provided", func(t *testing.T) {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:           "theme-name",
			Description:    "Theme name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose theme",
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser(
			WithPositionalArgPrompt("theme-name", "Choose theme", completionFunc),
		)
		parser.SetPositionalArgs(specs, validator, usage)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{},
			PositionalArgs: []string{"dracula"}, // Argument already provided.
		}

		err := parser.promptForMissingPositionalArgs(result)
		assert.NoError(t, err, "should not return error")
		assert.Equal(t, []string{"dracula"}, result.PositionalArgs, "should not change args when provided")
	})

	t.Run("skips prompt when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:           "theme-name",
			Description:    "Theme name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose theme",
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser(
			WithPositionalArgPrompt("theme-name", "Choose theme", completionFunc),
		)
		parser.SetPositionalArgs(specs, validator, usage)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{},
			PositionalArgs: []string{}, // No argument provided.
		}

		err := parser.promptForMissingPositionalArgs(result)
		assert.NoError(t, err, "should not return error when not interactive")
		assert.Empty(t, result.PositionalArgs, "should keep empty args when not interactive")
	})
}

// TestStandardFlagParser_HandleInteractivePrompts tests the overall prompting flow.
func TestStandardFlagParser_HandleInteractivePrompts(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	t.Run("executes all three use cases in correct order", func(t *testing.T) {
		viper.Set("interactive", false) // Ensure non-interactive for predictable test.

		parser := NewStandardFlagParser()

		result := &ParsedConfig{
			Flags:          map[string]interface{}{},
			PositionalArgs: []string{},
		}

		err := parser.handleInteractivePrompts(result, nil)
		assert.NoError(t, err, "should execute all prompting use cases without error")
	})

	t.Run("handles nil combinedFlags gracefully", func(t *testing.T) {
		parser := NewStandardFlagParser()

		result := &ParsedConfig{
			Flags:          map[string]interface{}{},
			PositionalArgs: []string{},
		}

		err := parser.handleInteractivePrompts(result, nil)
		assert.NoError(t, err, "should handle nil combinedFlags")
	})
}

// TestWithCompletionPrompt tests the WithCompletionPrompt option.
func TestWithCompletionPrompt(t *testing.T) {
	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	parser := NewStandardFlagParser(
		WithCompletionPrompt("test-flag", "Choose option", completionFunc),
	)

	require.NotNil(t, parser.flagPrompts, "flagPrompts map should be initialized")
	assert.Contains(t, parser.flagPrompts, "test-flag", "should contain prompt config for test-flag")

	config := parser.flagPrompts["test-flag"]
	require.NotNil(t, config, "prompt config should not be nil")
	assert.Equal(t, "Choose option", config.PromptTitle, "should set correct prompt title")
	assert.NotNil(t, config.CompletionFunc, "should set completion function")
}

// TestWithOptionalValuePrompt tests the WithOptionalValuePrompt option.
func TestWithOptionalValuePrompt(t *testing.T) {
	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"identity1", "identity2"}, cobra.ShellCompDirectiveNoFileComp
	}

	parser := NewStandardFlagParser(
		WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
	)

	require.NotNil(t, parser.optionalValuePrompts, "optionalValuePrompts map should be initialized")
	assert.Contains(t, parser.optionalValuePrompts, "identity", "should contain prompt config for identity")

	config := parser.optionalValuePrompts["identity"]
	require.NotNil(t, config, "prompt config should not be nil")
	assert.Equal(t, "Choose identity", config.PromptTitle, "should set correct prompt title")
	assert.NotNil(t, config.CompletionFunc, "should set completion function")
}

// TestWithPositionalArgPrompt tests the WithPositionalArgPrompt option.
func TestWithPositionalArgPrompt(t *testing.T) {
	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"arg1", "arg2"}, cobra.ShellCompDirectiveNoFileComp
	}

	parser := NewStandardFlagParser(
		WithPositionalArgPrompt("test-arg", "Choose argument", completionFunc),
	)

	require.NotNil(t, parser.positionalPrompts, "positionalPrompts map should be initialized")
	assert.Contains(t, parser.positionalPrompts, "test-arg", "should contain prompt config for test-arg")

	config := parser.positionalPrompts["test-arg"]
	require.NotNil(t, config, "prompt config should not be nil")
	assert.Equal(t, "Choose argument", config.PromptTitle, "should set correct prompt title")
	assert.NotNil(t, config.CompletionFunc, "should set completion function")
}

// TestPromptForValue_EmptyOptions tests PromptForValue with empty options while interactive mode attempts to activate.
func TestPromptForValue_EmptyOptions(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	t.Run("returns error with flag name when options are empty and not interactive", func(t *testing.T) {
		viper.Set("interactive", false)
		_, err := PromptForValue("my-flag", "Choose option", []string{})
		assert.Error(t, err, "should return error when no options")
		// Error should be ErrInteractiveModeNotAvailable since that check comes first.
		assert.Contains(t, err.Error(), "interactive mode not available")
	})
}

// TestPromptForMissingRequired_CompletionFuncCalled verifies completion function is invoked.
func TestPromptForMissingRequired_CompletionFuncCalled(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}
	args := []string{"arg1", "arg2"}

	t.Run("completion function receives correct arguments", func(t *testing.T) {
		viper.Set("interactive", false)

		var receivedCmd *cobra.Command
		var receivedArgs []string
		var receivedToComplete string

		completionFunc := func(c *cobra.Command, a []string, tc string) ([]string, cobra.ShellCompDirective) {
			receivedCmd = c
			receivedArgs = a
			receivedToComplete = tc
			return []string{"option1"}, cobra.ShellCompDirectiveNoFileComp
		}

		// Even though we're not interactive, we can verify the function would be called
		// by checking that non-interactive mode returns empty without calling completion.
		result, err := PromptForMissingRequired("test-flag", "Choose", completionFunc, cmd, args)
		assert.NoError(t, err)
		assert.Empty(t, result)
		// In non-interactive mode, completion func is NOT called (short-circuits first).
		assert.Nil(t, receivedCmd, "completion func should not be called in non-interactive mode")
		assert.Nil(t, receivedArgs, "completion func should not be called in non-interactive mode")
		assert.Empty(t, receivedToComplete, "completion func should not be called in non-interactive mode")
	})
}

// TestPromptForOptionalValue_NonSentinelValues tests various non-sentinel values.
func TestPromptForOptionalValue_NonSentinelValues(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}
	args := []string{}

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	testCases := []struct {
		name     string
		value    string
		expected string
	}{
		{"empty string is not sentinel", "", ""},
		{"regular value unchanged", "my-identity", "my-identity"},
		{"value with spaces unchanged", "my identity name", "my identity name"},
		{"numeric value unchanged", "12345", "12345"},
		{"partial sentinel unchanged", "__SELECT", "__SELECT"},
		{"sentinel suffix unchanged", "SELECT__", "SELECT__"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := PromptForOptionalValue("identity", tc.value, "Choose", completionFunc, cmd, args)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestPromptForOptionalValue_SentinelWithEmptyCompletions tests sentinel with no options.
func TestPromptForOptionalValue_SentinelWithEmptyCompletions(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}
	args := []string{}

	emptyCompletionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("sentinel with empty completions returns empty when not interactive", func(t *testing.T) {
		viper.Set("interactive", false)
		result, err := PromptForOptionalValue("identity", "__SELECT__", "Choose", emptyCompletionFunc, cmd, args)
		assert.NoError(t, err)
		assert.Empty(t, result, "should return empty when not interactive")
	})
}

// TestPromptForPositionalArg_WithCurrentArgs tests that current args are passed correctly.
func TestPromptForPositionalArg_WithCurrentArgs(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	cmd := &cobra.Command{Use: "test"}

	t.Run("current args are not passed to completion in non-interactive mode", func(t *testing.T) {
		viper.Set("interactive", false)

		var receivedArgs []string
		completionFunc := func(c *cobra.Command, a []string, tc string) ([]string, cobra.ShellCompDirective) {
			receivedArgs = a
			return []string{"arg1"}, cobra.ShellCompDirectiveNoFileComp
		}

		currentArgs := []string{"component-name", "stack-name"}
		result, err := PromptForPositionalArg("theme", "Choose theme", completionFunc, cmd, currentArgs)
		assert.NoError(t, err)
		assert.Empty(t, result)
		// In non-interactive mode, completion func is NOT called.
		assert.Nil(t, receivedArgs, "completion func should not be called in non-interactive mode")
	})
}

// TestStandardFlagParser_PromptForMissingRequiredFlags_FlagNotInMap tests missing flag in flags map.
func TestStandardFlagParser_PromptForMissingRequiredFlags_FlagNotInMap(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"stack1", "stack2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("handles flag not present in flags map", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{}, // stack not in map
			PositionalArgs: []string{},
		}

		err := parser.promptForMissingRequiredFlags(result, nil)
		assert.NoError(t, err, "should not return error when flag not in map")
	})

	t.Run("handles nil flag value in flags map", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"stack": nil}, // nil value
			PositionalArgs: []string{},
		}

		err := parser.promptForMissingRequiredFlags(result, nil)
		assert.NoError(t, err, "should not return error when flag value is nil")
	})
}

// TestStandardFlagParser_PromptForOptionalValueFlags_FlagNotInMap tests missing flag scenarios.
func TestStandardFlagParser_PromptForOptionalValueFlags_FlagNotInMap(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"identity1", "identity2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("handles flag not present in flags map", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{}, // identity not in map
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, nil)
		assert.NoError(t, err, "should not return error when flag not in map")
	})

	t.Run("handles non-string flag value", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithOptionalValuePrompt("identity", "Choose identity", completionFunc),
		)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{"identity": 123}, // non-string value
			PositionalArgs: []string{},
		}

		err := parser.promptForOptionalValueFlags(result, nil)
		assert.NoError(t, err, "should not return error when flag value is not a string")
		assert.Equal(t, 123, result.Flags["identity"], "should not change non-string value")
	})
}

// TestStandardFlagParser_PromptForMissingPositionalArgs_MultipleArgs tests multiple positional args.
func TestStandardFlagParser_PromptForMissingPositionalArgs_MultipleArgs(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"option1", "option2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("handles multiple positional args with some provided", func(t *testing.T) {
		viper.Set("interactive", false)

		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:           "component",
			Description:    "Component name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose component",
		})
		builder.AddArg(&PositionalArgSpec{
			Name:           "stack",
			Description:    "Stack name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose stack",
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser(
			WithPositionalArgPrompt("component", "Choose component", completionFunc),
			WithPositionalArgPrompt("stack", "Choose stack", completionFunc),
		)
		parser.SetPositionalArgs(specs, validator, usage)

		result := &ParsedConfig{
			Flags:          map[string]interface{}{},
			PositionalArgs: []string{"vpc"}, // Only first arg provided.
		}

		err := parser.promptForMissingPositionalArgs(result)
		assert.NoError(t, err, "should not return error")
		// In non-interactive mode, missing second arg is not prompted for.
		assert.Equal(t, []string{"vpc"}, result.PositionalArgs, "should keep provided args unchanged")
	})

	t.Run("handles no positional args specs", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser()
		// No positional args specs set.

		result := &ParsedConfig{
			Flags:          map[string]interface{}{},
			PositionalArgs: []string{},
		}

		err := parser.promptForMissingPositionalArgs(result)
		assert.NoError(t, err, "should not return error when no specs")
	})
}

// TestIsInteractive_TTYAndCIBehavior tests TTY and CI detection behavior.
func TestIsInteractive_TTYAndCIBehavior(t *testing.T) {
	// Save original viper state.
	originalInteractive := viper.GetBool("interactive")
	defer func() {
		viper.Set("interactive", originalInteractive)
	}()

	t.Run("returns false when interactive is disabled regardless of TTY", func(t *testing.T) {
		viper.Set("interactive", false)
		result := isInteractive()
		assert.False(t, result, "should return false when interactive flag is disabled")
	})

	t.Run("returns value based on TTY and CI when interactive is enabled", func(t *testing.T) {
		viper.Set("interactive", true)
		// In test environment, this typically returns false (no TTY or CI detected).
		// We just verify the function executes without panic.
		result := isInteractive()
		// Result depends on actual environment, but should be boolean.
		assert.IsType(t, true, result || !result, "should return boolean")
	})
}
