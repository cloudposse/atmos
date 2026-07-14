package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCPServerConfigTransportType(t *testing.T) {
	assert.Equal(t, MCPTransportStdio, MCPServerConfig{Command: "uvx"}.TransportType())
	assert.Equal(t, MCPTransportHTTP, MCPServerConfig{URL: "https://example.com/mcp"}.TransportType())
	assert.Equal(t, MCPTransportHTTP, MCPServerConfig{Type: " HTTP "}.TransportType())
}
