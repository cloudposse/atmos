package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShowCommand_Flags(t *testing.T) {
	// Test that show command has required flags.
	formatFlag := showCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "text", formatFlag.DefValue)
}

func TestShowCommand_BasicProperties(t *testing.T) {
	assert.Equal(t, "show [version]", showCmd.Use)
	assert.NotEmpty(t, showCmd.Short)
	assert.NotEmpty(t, showCmd.Long)
	assert.NotEmpty(t, showCmd.Example)
	assert.NotNil(t, showCmd.RunE)
	assert.NotNil(t, showCmd.Args)
}
