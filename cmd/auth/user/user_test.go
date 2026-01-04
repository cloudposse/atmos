package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthUserCommand_Structure(t *testing.T) {
	assert.Equal(t, "user", AuthUserCmd.Use)
	assert.NotEmpty(t, AuthUserCmd.Short)
	assert.NotEmpty(t, AuthUserCmd.Long)

	// User command should have subcommands.
	subcommands := AuthUserCmd.Commands()
	assert.Greater(t, len(subcommands), 0)

	// Get subcommand names.
	subcommandNames := make([]string, len(subcommands))
	for i, cmd := range subcommands {
		subcommandNames[i] = cmd.Name()
	}

	// Verify expected subcommands exist.
	assert.Contains(t, subcommandNames, "configure")
}

func TestAuthUserCommand_ShortDescription(t *testing.T) {
	// Verify the short description is set.
	assert.NotEmpty(t, AuthUserCmd.Short)
}

func TestAuthUserCommand_LongDescription(t *testing.T) {
	assert.NotEmpty(t, AuthUserCmd.Long)
}
