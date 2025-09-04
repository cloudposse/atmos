package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnsibleVaultCmd(t *testing.T) {
	// Test that the command is properly configured
	assert.Equal(t, "vault", ansibleVaultCmd.Use)
	assert.Contains(t, ansibleVaultCmd.Short, "Encrypt")
	assert.Contains(t, ansibleVaultCmd.Long, "Ansible Vault")
	assert.True(t, ansibleVaultCmd.FParseErrWhitelist.UnknownFlags)
	assert.NotNil(t, ansibleVaultCmd.RunE)
}
