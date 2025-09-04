package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnsibleInventoryCmd(t *testing.T) {
	// Test that the command is properly configured
	assert.Equal(t, "inventory", ansibleInventoryCmd.Use)
	assert.Contains(t, ansibleInventoryCmd.Short, "inventory")
	assert.Contains(t, ansibleInventoryCmd.Long, "inventory")
	assert.True(t, ansibleInventoryCmd.FParseErrWhitelist.UnknownFlags)
	assert.NotNil(t, ansibleInventoryCmd.RunE)
}
