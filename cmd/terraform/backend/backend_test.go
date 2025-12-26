package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSetAtmosConfig(t *testing.T) {
	// Save original value.
	original := atmosConfigPtr
	defer func() {
		atmosConfigPtr = original
	}()

	config := &schema.AtmosConfiguration{
		BasePath: "/test/path",
	}
	SetAtmosConfig(config)

	assert.Equal(t, config, atmosConfigPtr)
	assert.Equal(t, "/test/path", atmosConfigPtr.BasePath)
}

func TestGetBackendCommand(t *testing.T) {
	cmd := GetBackendCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "backend", cmd.Use)
	assert.Equal(t, "Manage Terraform state backends", cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify subcommands are registered.
	subcommands := cmd.Commands()
	assert.GreaterOrEqual(t, len(subcommands), 5, "should have at least 5 subcommands")

	// Verify specific subcommands exist by checking Use field prefix.
	// Note: createCmd.Use is "<component>" but all others start with their command name.
	subcommandUses := make(map[string]bool)
	for _, sub := range subcommands {
		subcommandUses[sub.Use] = true
	}

	// Verify we have the expected commands by their Use patterns.
	assert.True(t, subcommandUses["<component>"], "should have create subcommand (Use: '<component>')")
	assert.True(t, subcommandUses["list"], "should have list subcommand")
	assert.True(t, subcommandUses["describe <component>"], "should have describe subcommand")
	assert.True(t, subcommandUses["update <component>"], "should have update subcommand")
	assert.True(t, subcommandUses["delete <component>"], "should have delete subcommand")
}
