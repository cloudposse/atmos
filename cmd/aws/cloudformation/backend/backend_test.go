package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBackendCommand(t *testing.T) {
	cmd := GetBackendCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "backend", cmd.Use)
	assert.NotEmpty(t, cmd.Long)
}

func TestBackendCmd_HasAllSubcommands(t *testing.T) {
	names := make(map[string]bool)
	for _, c := range backendCmd.Commands() {
		names[c.Name()] = true
	}

	for _, want := range []string{"create", "update", "delete", "describe", "list"} {
		assert.True(t, names[want], "backend command should have %q subcommand", want)
	}
}
