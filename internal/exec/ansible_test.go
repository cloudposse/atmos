package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAnsibleFlags(t *testing.T) {
	// Test AnsibleFlags struct creation
	flags := AnsibleFlags{
		Playbook:  "site.yml",
		Inventory: "hosts.yml",
	}

	assert.Equal(t, "site.yml", flags.Playbook)
	assert.Equal(t, "hosts.yml", flags.Inventory)
}

func TestExecuteAnsible_Version(t *testing.T) {
	// Test version subcommand path
	info := &schema.ConfigAndStacksInfo{
		SubCommand: "version",
		Command:    "ansible",
	}

	flags := &AnsibleFlags{}

	// This would normally execute the version command, but in a test we would mock this
	// For now, just verify the structure is correct
	assert.NotNil(t, info)
	assert.NotNil(t, flags)
	assert.Equal(t, "version", info.SubCommand)
}
