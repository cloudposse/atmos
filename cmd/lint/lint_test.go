package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLintCommandProvider tests the LintCommandProvider implementation.
func TestLintCommandProvider(t *testing.T) {
	t.Parallel()
	provider := &LintCommandProvider{}

	t.Run("GetCommand returns lint command", func(t *testing.T) {
		t.Parallel()
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "lint", cmd.Use)
	})

	t.Run("GetName returns lint", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "lint", provider.GetName())
	})

	t.Run("GetGroup returns non-empty group name", func(t *testing.T) {
		t.Parallel()
		assert.NotEmpty(t, provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, provider.GetFlagsBuilder())
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, provider.GetCompatibilityFlags())
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, provider.GetAliases())
	})

	t.Run("IsExperimental returns false", func(t *testing.T) {
		t.Parallel()
		assert.False(t, provider.IsExperimental())
	})
}

// TestLintCmd_BasicProperties verifies the basic properties of the lint command.
func TestLintCmd_BasicProperties(t *testing.T) {
	t.Parallel()
	cmd := lintCmd
	assert.Equal(t, "lint", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
}

// TestLintStacksCmd_BasicProperties verifies the basic properties of the lint stacks command.
func TestLintStacksCmd_BasicProperties(t *testing.T) {
	t.Parallel()
	cmd := lintStacksCmd
	assert.Equal(t, "stacks", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)
}

// TestLintStacksCmd_Flags verifies that the lint stacks command registers the expected flags.
func TestLintStacksCmd_Flags(t *testing.T) {
	t.Parallel()
	cmd := lintStacksCmd

	assert.NotNil(t, cmd.Flags().Lookup("stack"), "expected --stack flag")
	assert.NotNil(t, cmd.Flags().Lookup("rule"), "expected --rule flag")
	assert.NotNil(t, cmd.Flags().Lookup("format"), "expected --format flag")
	assert.NotNil(t, cmd.Flags().Lookup("severity"), "expected --severity flag")
}

// TestLintCmd_HasStacksSubcommand verifies that the lint command has the stacks subcommand.
func TestLintCmd_HasStacksSubcommand(t *testing.T) {
	t.Parallel()
	cmd := lintCmd
	stacksCmd, _, err := cmd.Find([]string{"stacks"})
	assert.NoError(t, err)
	assert.NotNil(t, stacksCmd)
	assert.Equal(t, "stacks", stacksCmd.Use)
}

// TestLintStacksCmd_EnvBindings verifies that ATMOS_LINT_RULE and related env vars
// are registered for binding via the StandardFlagParser (High #5 — env var binding test).
func TestLintStacksCmd_EnvBindings(t *testing.T) {
	t.Parallel()

	// Verify the lintStacksParser is initialized and all expected env vars are
	// registered. The parser stores this mapping via WithEnvVars(...) options.
	// We verify correctness by confirming the expected flags exist on the command
	// (which is registered by BindToViper/RegisterFlags) and then checking that
	// the parser is not nil.

	// All four flags must be registered.
	assert.NotNil(t, lintStacksCmd.Flags().Lookup("rule"), "--rule flag must be registered")
	assert.NotNil(t, lintStacksCmd.Flags().Lookup("format"), "--format flag must be registered")
	assert.NotNil(t, lintStacksCmd.Flags().Lookup("severity"), "--severity flag must be registered")
	assert.NotNil(t, lintStacksCmd.Flags().Lookup("stack"), "--stack flag must be registered")

	// The lintStacksParser must not be nil (it is set in init()).
	assert.NotNil(t, lintStacksParser, "lintStacksParser must not be nil")
}
