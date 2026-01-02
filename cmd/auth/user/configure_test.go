package user

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthUserConfigureCommand_Structure(t *testing.T) {
	assert.Equal(t, "configure", authUserConfigureCmd.Use)
	assert.NotEmpty(t, authUserConfigureCmd.Short)
	assert.NotEmpty(t, authUserConfigureCmd.Long)
	assert.NotNil(t, authUserConfigureCmd.RunE)
}

func TestAuthUserConfigureCommand_FParseErrWhitelist(t *testing.T) {
	// Verify FParseErrWhitelist is configured.
	assert.False(t, authUserConfigureCmd.FParseErrWhitelist.UnknownFlags)
}

func TestAuthUserConfigureCommand_ShortDescription(t *testing.T) {
	assert.Contains(t, authUserConfigureCmd.Short, "Configure")
}

func TestAuthUserConfigureCommand_LongDescription(t *testing.T) {
	assert.Contains(t, authUserConfigureCmd.Long, "Configure")
}

func TestAuthUserConfigureCommand_IsSubcommandOfUser(t *testing.T) {
	// Verify configure is a subcommand of user.
	found := false
	for _, cmd := range AuthUserCmd.Commands() {
		if cmd.Name() == "configure" {
			found = true
			break
		}
	}
	assert.True(t, found, "configure should be a subcommand of user")
}
