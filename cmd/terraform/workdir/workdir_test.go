package workdir

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWorkdirCommand(t *testing.T) {
	cmd := GetWorkdirCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "workdir", cmd.Use)
	assert.Equal(t, "Manage component working directories", cmd.Short)

	// Check subcommands are registered.
	subcommands := cmd.Commands()
	subcommandNames := make([]string, len(subcommands))
	for i, sub := range subcommands {
		subcommandNames[i] = sub.Name()
	}

	assert.Contains(t, subcommandNames, "list")
	assert.Contains(t, subcommandNames, "describe")
	assert.Contains(t, subcommandNames, "show")
	assert.Contains(t, subcommandNames, "clean")
}
