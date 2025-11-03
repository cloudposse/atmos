package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSchemaCmd_FlagParsing(t *testing.T) {
	_ = NewTestKit(t)
	cmd := ValidateSchemaCmd

	// Ensure command is initialized.
	assert.NotNil(t, cmd, "ValidateSchemaCmd should not be nil")

	// Test that the schemas-atmos-manifest flag exists and has the correct properties.
	flag := cmd.Flags().Lookup("schemas-atmos-manifest")
	if flag == nil {
		flag = cmd.PersistentFlags().Lookup("schemas-atmos-manifest")
	}
	assert.NotNil(t, flag, "schemas-atmos-manifest flag should be registered")
	if flag != nil {
		assert.Equal(t, "string", flag.Value.Type())
		assert.Equal(t, "", flag.DefValue)
		assert.Equal(t, "Path to Atmos manifest JSON Schema", flag.Usage)
	}
}

func TestValidateSchemaCmd_UnknownFlags(t *testing.T) {
	_ = NewTestKit(t)
	cmd := ValidateSchemaCmd

	// Verify that unknown flags are not allowed.
	assert.False(t, cmd.FParseErrWhitelist.UnknownFlags)
}
