package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRegisterMCPTools_NoServers(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{}

	mgr, err := RegisterMCPTools(registry, atmosConfig, nil, nil)
	require.NoError(t, err)
	assert.Nil(t, mgr, "no manager when no servers configured")
	assert.Equal(t, 0, registry.Count())
}

func TestRegisterMCPTools_InvalidConfig(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.MCP.Servers = map[string]schema.MCPServerConfig{
		"bad": {Command: ""}, // Empty command.
	}

	_, err := RegisterMCPTools(registry, atmosConfig, nil, nil)
	require.Error(t, err)
}

func TestRegisterMCPTools_FailedStart(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.MCP.Servers = map[string]schema.MCPServerConfig{
		"bad-server": {Command: "nonexistent-binary-xyz-123"},
	}

	// Should not return error — failed starts are logged as warnings.
	mgr, err := RegisterMCPTools(registry, atmosConfig, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, mgr)
	// No tools registered since the server failed to start.
	assert.Equal(t, 0, registry.Count())
}

func TestRegisterMCPTools_FailedStart_ContinuesOtherServers(t *testing.T) {
	registry := tools.NewRegistry()
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.MCP.Servers = map[string]schema.MCPServerConfig{
		"bad-server":   {Command: "nonexistent-binary-xyz-123"},
		"bad-server-2": {Command: "another-nonexistent-binary-abc-456"},
	}

	// Should not return error — failed starts are logged as warnings.
	// Both servers fail but neither prevents the other from being attempted.
	mgr, err := RegisterMCPTools(registry, atmosConfig, nil, nil)
	require.NoError(t, err)
	assert.NotNil(t, mgr)
	// No tools registered since both servers failed to start.
	assert.Equal(t, 0, registry.Count())
	// Manager still holds both sessions.
	assert.Len(t, mgr.List(), 2)
}
