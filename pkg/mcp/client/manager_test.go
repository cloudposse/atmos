package client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewManager_EmptyIntegrations(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{})
	require.NoError(t, err)
	assert.Empty(t, mgr.List())
}

func TestNewManager_ValidConfig(t *testing.T) {
	integrations := map[string]schema.MCPIntegrationConfig{
		"server-a": {Command: "echo", Description: "Test A"},
		"server-b": {Command: "cat", Description: "Test B"},
	}
	mgr, err := NewManager(integrations)
	require.NoError(t, err)

	sessions := mgr.List()
	assert.Len(t, sessions, 2)
	// Sorted by name.
	assert.Equal(t, "server-a", sessions[0].Name())
	assert.Equal(t, "server-b", sessions[1].Name())
}

func TestNewManager_InvalidConfig(t *testing.T) {
	integrations := map[string]schema.MCPIntegrationConfig{
		"bad": {Command: ""}, // Empty command.
	}
	_, err := NewManager(integrations)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationCommandEmpty)
}

func TestManager_Get_NotFound(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{
		"exists": {Command: "echo"},
	})
	require.NoError(t, err)

	_, err = mgr.Get("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationNotFound)
}

func TestManager_Get_Found(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{
		"test": {Command: "echo", Description: "Test server"},
	})
	require.NoError(t, err)

	session, err := mgr.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "test", session.Name())
	assert.Equal(t, StatusStopped, session.Status())
}

func TestManager_Start_NotFound(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{})
	require.NoError(t, err)

	err = mgr.Start(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationNotFound)
}

func TestManager_Stop_NotFound(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{})
	require.NoError(t, err)

	err = mgr.Stop("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMCPIntegrationNotFound)
}

func TestManager_StopAll_NoRunning(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{
		"a": {Command: "echo"},
		"b": {Command: "cat"},
	})
	require.NoError(t, err)

	// No sessions are running, so StopAll should succeed.
	err = mgr.StopAll()
	require.NoError(t, err)
}

func TestManager_List_Sorted(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{
		"zulu":  {Command: "echo"},
		"alpha": {Command: "echo"},
		"mike":  {Command: "echo"},
	})
	require.NoError(t, err)

	sessions := mgr.List()
	assert.Len(t, sessions, 3)
	assert.Equal(t, "alpha", sessions[0].Name())
	assert.Equal(t, "mike", sessions[1].Name())
	assert.Equal(t, "zulu", sessions[2].Name())
}

func TestManager_Test_NotFound(t *testing.T) {
	mgr, err := NewManager(map[string]schema.MCPIntegrationConfig{})
	require.NoError(t, err)

	result := mgr.Test(context.Background(), "nonexistent")
	require.Error(t, result.Error)
	assert.ErrorIs(t, result.Error, errUtils.ErrMCPIntegrationNotFound)
	assert.False(t, result.ServerStarted)
}
