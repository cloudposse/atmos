package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnsibleVersionCmd(t *testing.T) {
	// Test that the command is properly configured
	assert.Equal(t, "version", ansibleVersionCmd.Use)
	assert.Contains(t, ansibleVersionCmd.Short, "version")
	assert.Contains(t, ansibleVersionCmd.Long, "version")
	assert.True(t, ansibleVersionCmd.FParseErrWhitelist.UnknownFlags)
	assert.NotNil(t, ansibleVersionCmd.RunE)
}
