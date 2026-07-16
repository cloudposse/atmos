package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackValidateCmd_RegisteredUnderStack(t *testing.T) {
	found := false
	for _, c := range stackCmd.Commands() {
		if c.Name() == "validate" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected \"validate\" to be registered as a subcommand of \"stack\"")
}

func TestStackValidateCmd_HasSchemaOverrideFlag(t *testing.T) {
	// The alias must accept the same schema-override flag as `atmos validate
	// stacks`, which the shared executor reads from the command's flag set.
	flag := stackValidateCmd.PersistentFlags().Lookup("schemas-atmos-manifest")
	require.NotNil(t, flag, "expected the schemas-atmos-manifest flag to be defined")
}

// The end-to-end behavior (delegating to `atmos validate stacks`) requires the
// root command's inherited global flags, so it is exercised through the CLI
// integration tests rather than by invoking the bare subcommand's RunE here.
