package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNoServersConfiguredMessage(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{
			name:    "mcp.enabled true points straight at add self",
			enabled: true,
			want: "No MCP servers configured. Add servers under `mcp.servers` in `atmos.yaml`." +
				" To use Atmos's own tools via MCP, run `atmos mcp add self`.",
		},
		{
			name:    "mcp.enabled false points at atmos config set first",
			enabled: false,
			want: "No MCP servers configured. Add servers under `mcp.servers` in `atmos.yaml`." +
				" To use Atmos's own tools via MCP, run `atmos config set mcp.enabled true` then `atmos mcp add self`.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, noServersConfiguredMessage(tt.enabled))
		})
	}
}

func TestAtmosProNudge(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		wantEmpty   bool
	}{
		{
			name:        "no workspace id configured, no nudge",
			atmosConfig: &schema.AtmosConfiguration{},
			wantEmpty:   true,
		},
		{
			name: "workspace id configured, atmos-pro not added, nudges",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{Pro: schema.ProSettings{WorkspaceID: "ws-123"}},
			},
			wantEmpty: false,
		},
		{
			name: "workspace id configured, atmos-pro already added by default name, no nudge",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{Pro: schema.ProSettings{WorkspaceID: "ws-123"}},
				MCP: schema.MCPSettings{Servers: map[string]schema.MCPServerConfig{
					"atmos-pro": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
				}},
			},
			wantEmpty: true,
		},
		{
			name: "workspace id configured, atmos-pro added under a renamed key, no nudge (matched by URL)",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{Pro: schema.ProSettings{WorkspaceID: "ws-123"}},
				MCP: schema.MCPSettings{Servers: map[string]schema.MCPServerConfig{
					"my-pro-server": {Type: schema.MCPTransportHTTP, URL: "https://atmos-pro.com/mcp"},
				}},
			},
			wantEmpty: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := atmosProNudge(tt.atmosConfig)
			if tt.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.NotEmpty(t, got)
				assert.Contains(t, got, "atmos mcp add atmos-pro")
			}
		})
	}
}
