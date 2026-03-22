package client

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// BridgedTool wraps an external MCP tool as an Atmos-compatible tool.
// It namespaces the tool name with the server name to avoid conflicts.
type BridgedTool struct {
	serverName string
	mcpTool    *mcpsdk.Tool
	session    *Session
}

// NewBridgedTool creates a new BridgedTool from an MCP tool definition.
func NewBridgedTool(serverName string, tool *mcpsdk.Tool, session *Session) *BridgedTool {
	return &BridgedTool{
		serverName: serverName,
		mcpTool:    tool,
		session:    session,
	}
}

// Name returns the namespaced tool name (e.g., "aws-eks.list_clusters").
func (t *BridgedTool) Name() string {
	return t.serverName + "." + t.mcpTool.Name
}

// Description returns the tool's description from the MCP server.
func (t *BridgedTool) Description() string {
	return t.mcpTool.Description
}

// ServerName returns the integration server name.
func (t *BridgedTool) ServerName() string {
	return t.serverName
}

// OriginalName returns the tool name without the server prefix.
func (t *BridgedTool) OriginalName() string {
	return t.mcpTool.Name
}

// RequiresPermission returns true — external tools should require permission by default.
func (t *BridgedTool) RequiresPermission() bool {
	return true
}

// IsRestricted returns false.
func (t *BridgedTool) IsRestricted() bool {
	return false
}

// Execute calls the tool on the external MCP server and returns the text result.
func (t *BridgedTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	result, err := t.session.CallTool(ctx, t.mcpTool.Name, params)
	if err != nil {
		return "", fmt.Errorf("MCP tool %q execution failed: %w", t.Name(), err)
	}
	return ExtractTextContent(result), nil
}

// ExtractTextContent extracts text from an MCP CallToolResult.
func ExtractTextContent(result *mcpsdk.CallToolResult) string {
	if result == nil {
		return ""
	}
	var parts []string
	for _, content := range result.Content {
		if tc, ok := content.(*mcpsdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// BridgeTools creates BridgedTools for all tools from a session.
func BridgeTools(session *Session) []*BridgedTool {
	tools := session.Tools()
	bridged := make([]*BridgedTool, len(tools))
	for i, tool := range tools {
		bridged[i] = NewBridgedTool(session.Name(), tool, session)
	}
	return bridged
}
