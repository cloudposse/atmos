package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuthValidateCommand_Structure(t *testing.T) {
	assert.Equal(t, "validate", authValidateCmd.Use)
	assert.NotEmpty(t, authValidateCmd.Short)
	assert.NotEmpty(t, authValidateCmd.Long)
	assert.NotNil(t, authValidateCmd.RunE)

	// Check verbose flag exists.
	verboseFlag := authValidateCmd.Flags().Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "v", verboseFlag.Shorthand)
}

func TestValidateParser_Initialization(t *testing.T) {
	// validateParser should be initialized in init().
	assert.NotNil(t, validateParser)
}

func TestAuthValidateCommand_FParseErrWhitelist(t *testing.T) {
	// Verify FParseErrWhitelist is configured.
	assert.False(t, authValidateCmd.FParseErrWhitelist.UnknownFlags)
}

func TestAuthValidateCommand_VerboseFlagDefault(t *testing.T) {
	// Verbose flag should default to false.
	verboseFlag := authValidateCmd.Flags().Lookup("verbose")
	assert.NotNil(t, verboseFlag)
	assert.Equal(t, "false", verboseFlag.DefValue)
}
