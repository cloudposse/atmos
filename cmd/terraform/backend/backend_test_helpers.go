package backend

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/flags"
)

// commandTestParams holds parameters for testing backend command structure.
type commandTestParams struct {
	cmd           *cobra.Command
	parser        *flags.StandardParser
	expectedUse   string
	expectedShort string
	requiredFlags []string
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

	t.Run("command requires exactly one argument", func(t *testing.T) {
		// The Args field should be set to cobra.ExactArgs(1).
		assert.NotNil(t, params.cmd.Args)

		// Test with no args.
		err := params.cmd.Args(params.cmd, []string{})
		assert.Error(t, err, "should error with no arguments")

		// Test with one arg.
		err = params.cmd.Args(params.cmd, []string{"vpc"})
		assert.NoError(t, err, "should accept exactly one argument")

		// Test with multiple args.
		err = params.cmd.Args(params.cmd, []string{"vpc", "extra"})
		assert.Error(t, err, "should error with multiple arguments")
	})
}
