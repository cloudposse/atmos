package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestBridgedTool_Name(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters", Description: "List EKS clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "aws-eks.list_clusters", bt.Name())
}

func TestBridgedTool_Description(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters", Description: "List EKS clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "List EKS clusters", bt.Description())
}

func TestBridgedTool_ServerName(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "aws-eks", bt.ServerName())
}

func TestBridgedTool_OriginalName(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "list_clusters"}
	bt := NewBridgedTool("aws-eks", tool, nil)

	assert.Equal(t, "list_clusters", bt.OriginalName())
}

func TestBridgedTool_RequiresPermission(t *testing.T) {
	bt := NewBridgedTool("test", &mcpsdk.Tool{}, nil)
	assert.True(t, bt.RequiresPermission())
}

func TestBridgedTool_IsRestricted(t *testing.T) {
	bt := NewBridgedTool("test", &mcpsdk.Tool{}, nil)
	assert.False(t, bt.IsRestricted())
}

func TestExtractTextContent_Nil(t *testing.T) {
	assert.Equal(t, "", ExtractTextContent(nil))
}

func TestExtractTextContent_EmptyContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{}
	assert.Equal(t, "", ExtractTextContent(result))
}

func TestExtractTextContent_TextContent(t *testing.T) {
	result := &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{
			&mcpsdk.TextContent{Text: "Hello"},
			&mcpsdk.TextContent{Text: "World"},
		},
	}
	assert.Equal(t, "Hello\nWorld", ExtractTextContent(result))
}

func TestBridgeTools(t *testing.T) {
	cfg := &ParsedConfig{Name: "test-server", Command: "echo"}
	session := NewSession(cfg)

	// Manually set tools on the session for testing.
	session.tools = []*mcpsdk.Tool{
		{Name: "tool_a", Description: "Tool A"},
		{Name: "tool_b", Description: "Tool B"},
	}

	bridged := BridgeTools(session)
	assert.Len(t, bridged, 2)
	assert.Equal(t, "test-server.tool_a", bridged[0].Name())
	assert.Equal(t, "test-server.tool_b", bridged[1].Name())
}

func TestBridgeTools_Empty(t *testing.T) {
	cfg := &ParsedConfig{Name: "test", Command: "echo"}
	session := NewSession(cfg)

	bridged := BridgeTools(session)
	assert.Empty(t, bridged)
}
