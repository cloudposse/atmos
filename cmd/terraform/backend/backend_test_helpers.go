package backend

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
)

// commandTestParams holds parameters for testing backend command structure.
type commandTestParams struct {
	cmd              *cobra.Command
	parser           *flags.StandardParser
	expectedUse      string
	expectedShort    string
	requiredFlags    []string
	hasPositionalArg bool // Whether the command has a positional arg (with prompting).
}

// testCommandStructure is a helper function to test common command structure patterns.
// It reduces code duplication across backend command tests.
func testCommandStructure(t *testing.T, params commandTestParams) {
	t.Helper()

	t.Run("command is properly configured", func(t *testing.T) {
		assert.NotNil(t, params.cmd)
		assert.Equal(t, params.expectedUse, params.cmd.Use)
		assert.Equal(t, params.expectedShort, params.cmd.Short)
		assert.NotEmpty(t, params.cmd.Long)
		assert.NotEmpty(t, params.cmd.Example)
		assert.False(t, params.cmd.DisableFlagParsing)
	})

	t.Run("parser is configured with required flags", func(t *testing.T) {
		assert.NotNil(t, params.parser)

		for _, flagName := range params.requiredFlags {
			flag := params.cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "%s flag should be registered", flagName)
		}

		// Verify stack flag exists (common to all commands).
		stackFlag := params.cmd.Flags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack flag should be registered")

		// Verify identity flag exists (common to all commands).
		identityFlag := params.cmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag, "identity flag should be registered")
	})

	// Only test arg validation for commands without prompt-aware validation.
	// Commands with prompting allow 0 args and will prompt for missing values.
	if params.hasPositionalArg {
		t.Run("command has prompt-aware arg validation", func(t *testing.T) {
			// With prompting, the Args validator is set by SetPositionalArgs.
			// It should allow 0 args (for prompting) and 1 arg (when provided).
			assert.NotNil(t, params.cmd.Args)

			// 0 args is allowed (will trigger prompt in interactive mode).
			err := params.cmd.Args(params.cmd, []string{})
			assert.NoError(t, err, "should allow 0 arguments (prompting enabled)")

			// 1 arg is allowed.
			err = params.cmd.Args(params.cmd, []string{"vpc"})
			assert.NoError(t, err, "should accept one argument")

			// Multiple args should error.
			err = params.cmd.Args(params.cmd, []string{"vpc", "extra"})
			assert.Error(t, err, "should error with multiple arguments")
		})
	}
}
