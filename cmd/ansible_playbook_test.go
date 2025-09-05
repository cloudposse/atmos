package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnsiblePlaybookCmd(t *testing.T) {
	// Test that the command is properly configured
	assert.Equal(t, "playbook", ansiblePlaybookCmd.Use)
	assert.Contains(t, ansiblePlaybookCmd.Short, "Ansible playbook")
	assert.Contains(t, ansiblePlaybookCmd.Long, "Ansible playbook")
	assert.True(t, ansiblePlaybookCmd.FParseErrWhitelist.UnknownFlags)
	assert.NotNil(t, ansiblePlaybookCmd.RunE)
}
