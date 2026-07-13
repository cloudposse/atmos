package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newParsedCommand builds a command with a couple of flags and executes it so
// Cobra populates ArgsLenAtDash from a real parse, capturing what RunE sees.
func newParsedCommand(t *testing.T, argv []string) (*cobra.Command, []string) {
	t.Helper()

	var runeArgs []string
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			runeArgs = args
			return nil
		},
	}
	cmd.Flags().StringP("stack", "s", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.SetArgs(argv)
	require.NoError(t, cmd.Execute())
	return cmd, runeArgs
}

func TestSplitArgsAtDash(t *testing.T) {
	t.Run("nil cmd returns all args as positional", func(t *testing.T) {
		positional, separated := SplitArgsAtDash(nil, []string{"component", "--check"})
		assert.Equal(t, []string{"component", "--check"}, positional)
		assert.Nil(t, separated)
	})

	t.Run("no dash returns all args as positional", func(t *testing.T) {
		cmd, args := newParsedCommand(t, []string{"component", "-s", "dev"})
		positional, separated := SplitArgsAtDash(cmd, args)
		assert.Equal(t, []string{"component"}, positional)
		assert.Nil(t, separated)
	})

	t.Run("dash with real parsed command splits at separator", func(t *testing.T) {
		cmd, args := newParsedCommand(t, []string{"component", "-s", "dev", "--", "--check", "--tags", "net"})
		positional, separated := SplitArgsAtDash(cmd, args)
		assert.Equal(t, []string{"component"}, positional)
		assert.Equal(t, []string{"--check", "--tags", "net"}, separated)
	})

	t.Run("dash at position 0 returns empty positional", func(t *testing.T) {
		cmd, args := newParsedCommand(t, []string{"--", "--check"})
		positional, separated := SplitArgsAtDash(cmd, args)
		assert.Empty(t, positional)
		assert.Equal(t, []string{"--check"}, separated)
	})

	t.Run("dash index beyond slice length returns all args as positional", func(t *testing.T) {
		cmd, _ := newParsedCommand(t, []string{"a", "b", "--", "c"})
		// Pass a shorter slice than what Cobra parsed (ArgsLenAtDash is 2).
		positional, separated := SplitArgsAtDash(cmd, []string{"a"})
		assert.Equal(t, []string{"a"}, positional)
		assert.Nil(t, separated)
	})
}

func TestSeparatorAwareValidator(t *testing.T) {
	t.Run("nil validator returns nil", func(t *testing.T) {
		assert.Nil(t, SeparatorAwareValidator(nil))
	})

	t.Run("nil cmd validates the full slice", func(t *testing.T) {
		validator := SeparatorAwareValidator(cobra.ExactArgs(1))
		assert.NoError(t, validator(nil, []string{"component"}))
		assert.Error(t, validator(nil, []string{"comp1", "comp2"}))
	})

	t.Run("accepts pass-through args after dash", func(t *testing.T) {
		cmd, args := newParsedCommand(t, []string{"component", "--", "--check", "--tags", "net"})
		validator := SeparatorAwareValidator(cobra.ExactArgs(1))
		assert.NoError(t, validator(cmd, args))
	})

	t.Run("rejects missing positional before dash", func(t *testing.T) {
		cmd, args := newParsedCommand(t, []string{"--", "--check"})
		validator := SeparatorAwareValidator(cobra.ExactArgs(1))
		assert.ErrorContains(t, validator(cmd, args), "accepts 1 arg(s), received 0")
	})

	t.Run("rejects surplus positionals before dash", func(t *testing.T) {
		cmd, args := newParsedCommand(t, []string{"comp1", "comp2", "--", "--check"})
		validator := SeparatorAwareValidator(cobra.ExactArgs(1))
		assert.ErrorContains(t, validator(cmd, args), "accepts 1 arg(s), received 2")
	})
}

func TestPositionalArgsBuilderSeparatorAwareValidators(t *testing.T) {
	newBuilder := func(required bool) *PositionalArgsBuilder {
		b := NewPositionalArgsBuilder()
		b.AddArg(&PositionalArgSpec{Name: "component", Description: "c", Required: required, TargetField: "Component"})
		return b
	}

	t.Run("required arg validator ignores args after dash", func(t *testing.T) {
		_, validator, usage := newBuilder(true).Build()
		assert.Equal(t, "<component>", usage)

		cmd, args := newParsedCommand(t, []string{"component", "--", "--check"})
		assert.NoError(t, validator(cmd, args))

		cmd, args = newParsedCommand(t, []string{"--", "--check"})
		assert.Error(t, validator(cmd, args))
	})

	t.Run("optional arg validator ignores args after dash", func(t *testing.T) {
		_, validator, _ := newBuilder(false).Build()

		cmd, args := newParsedCommand(t, []string{"--", "--check", "--diff"})
		assert.NoError(t, validator(cmd, args))
	})

	t.Run("prompt-aware validator counts only pre-dash args", func(t *testing.T) {
		validator := newBuilder(true).GeneratePromptAwareValidator(true)

		cmd, args := newParsedCommand(t, []string{"component", "--", "--check", "--tags", "net"})
		assert.NoError(t, validator(cmd, args))

		// Missing component is allowed (prompts fill it in later).
		cmd, args = newParsedCommand(t, []string{"--", "--check"})
		assert.NoError(t, validator(cmd, args))

		// Surplus pre-dash positionals are still rejected.
		cmd, args = newParsedCommand(t, []string{"comp1", "comp2", "--", "--check"})
		assert.ErrorContains(t, validator(cmd, args), "accepts at most 1 arg(s), received 2")
	})
}
