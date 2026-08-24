package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: if schema.MCPServerConfig.Command or .Identity is
// renamed or removed, this declaration fails the build before any test runs.
// Per CLAUDE.md: "Add compile-time sentinels for schema field references in tests".
var _ = schema.MCPServerConfig{
	Command:  "echo",
	Identity: "ci",
}

func TestResolveAuthProvider_NoIdentity_ReturnsNil(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo"},
	}
	assert.Nil(t, resolveAuthProvider(cfg))
}

func TestResolveAuthProvider_WithIdentity_ReturnsScopedProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo", Identity: "ci"},
	}
	provider := resolveAuthProvider(cfg)
	require.NotNil(t, provider)

	// Must be per-server aware so WithAuthManager will dispatch to ForServer.
	_, isPerServer := provider.(mcpclient.PerServerAuthProvider)
	assert.True(t, isPerServer, "resolved provider must implement PerServerAuthProvider")
}
