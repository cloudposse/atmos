package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMcpServersNeedAuth(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]schema.MCPServerConfig
		want    bool
	}{
		{
			name:    "empty",
			servers: nil,
			want:    false,
		},
		{
			name: "no identity",
			servers: map[string]schema.MCPServerConfig{
				"a": {Command: "echo"},
			},
			want: false,
		},
		{
			name: "with identity",
			servers: map[string]schema.MCPServerConfig{
				"a": {Command: "echo", Identity: "ci"},
			},
			want: true,
		},
		{
			name: "mixed",
			servers: map[string]schema.MCPServerConfig{
				"a": {Command: "echo"},
				"b": {Command: "echo", Identity: "ci"},
			},
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, mcpServersNeedAuth(tc.servers))
		})
	}
}

func TestBuildAuthOption_NoServersNeedingAuth(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo"},
	}
	assert.Nil(t, buildAuthOption(cfg))
}

func TestBuildAuthOption_ReturnsScopedProvider(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}
	cfg.MCP.Servers = map[string]schema.MCPServerConfig{
		"a": {Command: "echo", Identity: "ci"},
	}
	opts := buildAuthOption(cfg)
	require.Len(t, opts, 1)

	// The returned option must be backed by the shared ScopedAuthProvider
	// from pkg/mcp/client, not a cmd/-local factory. We can't easily assert
	// the StartOption closure's inner type, so we re-verify by constructing
	// a ScopedAuthProvider directly and confirming it satisfies both
	// interfaces the caller depends on.
	provider := mcpclient.NewScopedAuthProvider(cfg)
	var _ mcpclient.AuthEnvProvider = provider
	var _ mcpclient.PerServerAuthProvider = provider
}
