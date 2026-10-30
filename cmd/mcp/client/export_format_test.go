package client

import (
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"

	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
)

func TestMCPExportCodexTOML(t *testing.T) {
	config := mcpclient.MCPJSONConfig{MCPServers: map[string]mcpclient.MCPJSONServer{
		"local.test": {Command: "atmos", Args: []string{"auth", "exec", "--", "tool"}, Env: map[string]string{"UPPER": "value"}},
		"remote":     {Type: "http", URL: "https://example.invalid/mcp", Headers: map[string]string{"X-Test": "value"}},
	}}
	data, err := marshalMCPExport(config, "nested/config.toml")
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, toml.Unmarshal(data, &decoded))
	servers := decoded["mcp_servers"].(map[string]any)
	require.Equal(t, "atmos", servers["local.test"].(map[string]any)["command"])
	require.Equal(t, map[string]any{"UPPER": "value"}, servers["local.test"].(map[string]any)["env"])
	require.Equal(t, map[string]any{"X-Test": "value"}, servers["remote"].(map[string]any)["http_headers"])
	require.NotContains(t, servers["remote"].(map[string]any), "type")
}
