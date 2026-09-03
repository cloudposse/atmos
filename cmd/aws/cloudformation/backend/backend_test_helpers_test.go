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

// testCommandStructure is a helper function to test common command structure
// patterns, reducing duplication across backend command tests. Mirrors
// cmd/terraform/backend/backend_test_helpers.go's helper of the same name.
func testCommandStructure(t *testing.T, params *commandTestParams) {
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

		stackFlag := params.cmd.Flags().Lookup("stack")
		assert.NotNil(t, stackFlag, "stack flag should be registered")

		identityFlag := params.cmd.Flags().Lookup("identity")
		assert.NotNil(t, identityFlag, "identity flag should be registered")
	})

	if params.hasPositionalArg {
		t.Run("command has prompt-aware arg validation", func(t *testing.T) {
			assert.NotNil(t, params.cmd.Args)

			err := params.cmd.Args(params.cmd, []string{})
			assert.NoError(t, err, "should allow 0 arguments (prompting enabled)")

			err = params.cmd.Args(params.cmd, []string{"vpc"})
			assert.NoError(t, err, "should accept one argument")

			err = params.cmd.Args(params.cmd, []string{"vpc", "extra"})
			assert.Error(t, err, "should error with multiple arguments")
		})
	}
}
