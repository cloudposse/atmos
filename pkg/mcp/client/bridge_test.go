package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Verify BridgedTool implements tools.Tool at compile time.
var _ tools.Tool = (*BridgedTool)(nil)

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

func TestBridgedTool_Parameters_WithSchema(t *testing.T) {
	tool := &mcpsdk.Tool{
		Name: "test_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region": map[string]any{
					"type":        "string",
					"description": "AWS region",
				},
				"count": map[string]any{
					"type":        "integer",
					"description": "Number of items",
				},
				"verbose": map[string]any{
					"type": "boolean",
				},
			},
			"required": []any{"region"},
		},
	}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Len(t, params, 3)

	// Find region param.
	var regionParam *tools.Parameter
	for i := range params {
		if params[i].Name == "region" {
			regionParam = &params[i]
			break
		}
	}
	assert.NotNil(t, regionParam)
	assert.True(t, regionParam.Required)
	assert.Equal(t, tools.ParamTypeString, regionParam.Type)
	assert.Equal(t, "AWS region", regionParam.Description)
}

func TestBridgedTool_Parameters_NoSchema(t *testing.T) {
	tool := &mcpsdk.Tool{Name: "test"}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Nil(t, params)
}

func TestBridgedTool_Parameters_NoProperties(t *testing.T) {
	tool := &mcpsdk.Tool{
		Name:        "test",
		InputSchema: map[string]any{"type": "object"},
	}
	bt := NewBridgedTool("test", tool, nil)

	params := bt.Parameters()
	assert.Nil(t, params)
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

func TestMapJSONSchemaType(t *testing.T) {
	tests := []struct {
		input string
		want  tools.ParamType
	}{
		{"string", tools.ParamTypeString},
		{"integer", tools.ParamTypeInt},
		{"number", tools.ParamTypeInt},
		{"boolean", tools.ParamTypeBool},
		{"array", tools.ParamTypeArray},
		{"object", tools.ParamTypeObject},
		{"unknown", tools.ParamTypeString},
		{"", tools.ParamTypeString},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, mapJSONSchemaType(tt.input))
		})
	}
}
