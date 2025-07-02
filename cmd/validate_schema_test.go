package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSchemaCmd_FlagParsing(t *testing.T) {
	cmd := ValidateSchemaCmd

	// Test that the schemas-atmos-manifest flag exists and has the correct properties
	flag := cmd.PersistentFlags().Lookup("schemas-atmos-manifest")
	assert.NotNil(t, flag)
	assert.Equal(t, "string", flag.Value.Type())
	assert.Equal(t, "", flag.DefValue)
	assert.Equal(t, "Specifies the path to a JSON schema file used to validate the structure and content of the Atmos manifest file", flag.Usage)
}

func TestValidateSchemaCmd_UnknownFlags(t *testing.T) {
	cmd := ValidateSchemaCmd

	// Verify that unknown flags are not allowed
	assert.False(t, cmd.FParseErrWhitelist.UnknownFlags)
}
