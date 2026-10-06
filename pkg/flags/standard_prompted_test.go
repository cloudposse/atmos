package flags

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkFieldPrompted verifies the lazy-init helper backing PromptedFields tracking.
func TestMarkFieldPrompted(t *testing.T) {
	t.Run("initializes map on first use", func(t *testing.T) {
		result := &ParsedConfig{}
		markFieldPrompted(result, "stack")
		require.NotNil(t, result.PromptedFields)
		assert.True(t, result.PromptedFields["stack"])
	})

	t.Run("adds additional keys without clobbering existing ones", func(t *testing.T) {
		result := &ParsedConfig{PromptedFields: map[string]bool{"stack": true}}
		markFieldPrompted(result, "component")
		assert.True(t, result.PromptedFields["stack"], "existing entry must survive")
		assert.True(t, result.PromptedFields["component"])
	})
}

// TestStandardParser_BuildStandardOptions_PromptedFields verifies that
// StandardOptions.ComponentPrompted / StackPrompted are correctly derived from
// ParsedConfig.PromptedFields. This is the seam CodeRabbit's review flagged:
// StandardParser must surface which values were resolved via interactive
// prompt so callers (e.g. cmd/terraform/backend) can build a correct
// auth.ReExecContext without depending on a real TTY prompt actually running.
func TestStandardParser_BuildStandardOptions_PromptedFields(t *testing.T) {
	tests := []struct {
		name                      string
		promptedFields            map[string]bool
		expectedComponentPrompted bool
		expectedStackPrompted     bool
	}{
		{
			name:                      "nothing prompted",
			promptedFields:            nil,
			expectedComponentPrompted: false,
			expectedStackPrompted:     false,
		},
		{
			name:                      "only component prompted",
			promptedFields:            map[string]bool{"component": true},
			expectedComponentPrompted: true,
			expectedStackPrompted:     false,
		},
		{
			name:                      "only stack prompted",
			promptedFields:            map[string]bool{"stack": true},
			expectedComponentPrompted: false,
			expectedStackPrompted:     true,
		},
		{
			name:                      "both component and stack prompted",
			promptedFields:            map[string]bool{"component": true, "stack": true},
			expectedComponentPrompted: true,
			expectedStackPrompted:     true,
		},
		{
			name:                      "unrelated prompted field does not leak",
			promptedFields:            map[string]bool{"identity": true},
			expectedComponentPrompted: false,
			expectedStackPrompted:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &StandardParser{}
			parsedConfig := &ParsedConfig{
				Flags:          map[string]interface{}{"stack": "dev"},
				PromptedFields: tt.promptedFields,
			}

			opts := p.buildStandardOptions(parsedConfig, "vpc", "", "")

			assert.Equal(t, tt.expectedComponentPrompted, opts.ComponentPrompted)
			assert.Equal(t, tt.expectedStackPrompted, opts.StackPrompted)
			// Sanity: unrelated fields still resolve normally (purely additive change).
			assert.Equal(t, "vpc", opts.Component)
			assert.Equal(t, "dev", opts.Stack)
		})
	}
}

// TestStandardParser_Parse_NotPromptedWhenSuppliedOnCLI verifies the negative
// path end-to-end through the real Parse() pipeline: when component and stack
// are supplied directly on the CLI (positional arg + flag), ComponentPrompted
// and StackPrompted must be false, even though both flags have prompts
// configured. This guards against a regression where any prompt-configured
// field is marked prompted regardless of how its value was actually resolved.
func TestStandardParser_Parse_NotPromptedWhenSuppliedOnCLI(t *testing.T) {
	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"vpc", "eks"}, cobra.ShellCompDirectiveNoFileComp
	}

	argsBuilder := NewPositionalArgsBuilder()
	argsBuilder.AddArg(&PositionalArgSpec{
		Name:           "component",
		Description:    "Component name",
		Required:       true,
		TargetField:    "Component",
		CompletionFunc: completionFunc,
		PromptTitle:    "Choose a component",
	})
	specs, _, usage := argsBuilder.Build()

	parser := NewStandardParser(
		WithStringFlag("stack", "s", "", "Stack name"),
		WithCompletionPrompt("stack", "Choose a stack", completionFunc),
		WithPositionalArgPrompt("component", "Choose a component", completionFunc),
	)
	parser.SetPositionalArgs(specs, nil, usage)

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	v := viper.New()
	require.NoError(t, parser.BindToViper(v))
	require.NoError(t, cmd.Flags().Parse([]string{"--stack", "dev"}))
	require.NoError(t, parser.BindFlagsToViper(cmd, v))

	opts, err := parser.Parse(context.Background(), []string{"vpc", "--stack", "dev"})
	require.NoError(t, err)

	assert.Equal(t, "vpc", opts.Component)
	assert.Equal(t, "dev", opts.Stack)
	assert.False(t, opts.ComponentPrompted, "component supplied positionally must not be marked prompted")
	assert.False(t, opts.StackPrompted, "stack supplied via flag must not be marked prompted")
}

// TestStandardFlagParser_PromptForSingleMissingFlag_DoesNotMarkPromptedWhenSkipped
// extends the existing skip-path coverage in standard_test.go to assert that
// PromptedFields stays empty when promptForSingleMissingFlag returns early
// (value already present, or explicitly set to empty, or non-interactive).
func TestStandardFlagParser_PromptForSingleMissingFlag_DoesNotMarkPromptedWhenSkipped(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	defer viper.Set("interactive", originalInteractive)

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"stack1", "stack2"}, cobra.ShellCompDirectiveNoFileComp
	}

	t.Run("flag already has value", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithStringFlag("stack", "s", "", "Stack name"),
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{Flags: map[string]interface{}{"stack": "prod"}}

		require.NoError(t, parser.promptForSingleMissingFlag("stack", result, cmd.Flags()))
		assert.Empty(t, result.PromptedFields)
	})

	t.Run("not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := NewStandardFlagParser(
			WithStringFlag("stack", "s", "", "Stack name"),
			WithCompletionPrompt("stack", "Choose stack", completionFunc),
		)
		cmd := &cobra.Command{Use: "test"}
		parser.RegisterFlags(cmd)

		result := &ParsedConfig{Flags: map[string]interface{}{"stack": ""}}

		require.NoError(t, parser.promptForSingleMissingFlag("stack", result, cmd.Flags()))
		assert.Empty(t, result.PromptedFields)
	})
}

// TestStandardFlagParser_PromptForMissingPositionalArgs_DoesNotMarkPromptedWhenSkipped
// mirrors the flag-prompt skip coverage above for the positional-arg prompt path
// (Use Case 3, used by e.g. cmd/terraform/backend's "component" positional arg).
func TestStandardFlagParser_PromptForMissingPositionalArgs_DoesNotMarkPromptedWhenSkipped(t *testing.T) {
	originalInteractive := viper.GetBool("interactive")
	defer viper.Set("interactive", originalInteractive)

	completionFunc := func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"vpc", "eks"}, cobra.ShellCompDirectiveNoFileComp
	}

	buildParser := func() *StandardFlagParser {
		builder := NewPositionalArgsBuilder()
		builder.AddArg(&PositionalArgSpec{
			Name:           "component",
			Description:    "Component name",
			Required:       true,
			CompletionFunc: completionFunc,
			PromptTitle:    "Choose a component",
		})
		specs, validator, usage := builder.Build()

		parser := NewStandardFlagParser(
			WithPositionalArgPrompt("component", "Choose a component", completionFunc),
		)
		parser.SetPositionalArgs(specs, validator, usage)
		return parser
	}

	t.Run("argument already provided", func(t *testing.T) {
		parser := buildParser()
		result := &ParsedConfig{PositionalArgs: []string{"vpc"}}

		require.NoError(t, parser.promptForMissingPositionalArgs(result))
		assert.Empty(t, result.PromptedFields)
	})

	t.Run("not interactive", func(t *testing.T) {
		viper.Set("interactive", false)

		parser := buildParser()
		result := &ParsedConfig{PositionalArgs: []string{}}

		require.NoError(t, parser.promptForMissingPositionalArgs(result))
		assert.Empty(t, result.PromptedFields)
	})
}
