package client

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// BridgedTool wraps an external MCP tool to implement the Atmos tools.Tool interface.
// It namespaces the tool name with the server name to avoid conflicts.
type BridgedTool struct {
	serverName string
	mcpTool    *mcpsdk.Tool
	session    *Session
}

// Verify BridgedTool implements tools.Tool at compile time.
var _ tools.Tool = (*BridgedTool)(nil)

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

// Parameters extracts parameter definitions from the MCP tool's InputSchema.
func (t *BridgedTool) Parameters() []tools.Parameter {
	schema, ok := t.mcpTool.InputSchema.(map[string]any)
	if !ok {
		return nil
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}

	// Extract required fields.
	requiredSet := make(map[string]bool)
	if reqList, ok := schema["required"].([]any); ok {
		for _, r := range reqList {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	params := make([]tools.Parameter, 0, len(props))
	for name, propRaw := range props {
		prop, ok := propRaw.(map[string]any)
		if !ok {
			continue
		}

		p := tools.Parameter{
			Name:     name,
			Required: requiredSet[name],
		}

		if desc, ok := prop["description"].(string); ok {
			p.Description = desc
		}

		if typeName, ok := prop["type"].(string); ok {
			p.Type = mapJSONSchemaType(typeName)
		} else {
			p.Type = tools.ParamTypeString
		}

		params = append(params, p)
	}

	return params
}

// Execute calls the tool on the external MCP server and returns a tools.Result.
func (t *BridgedTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	result, err := t.session.CallTool(ctx, t.mcpTool.Name, params)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("MCP tool %q execution failed: %v", t.Name(), err),
			Error:   err,
		}, nil
	}

	output := ExtractTextContent(result)

	if result.IsError {
		return &tools.Result{
			Success: false,
			Output:  output,
			Error:   fmt.Errorf("%w: server %q, tool %q: %s", errUtils.ErrMCPServerToolError, t.serverName, t.mcpTool.Name, output),
		}, nil
	}

	return &tools.Result{
		Success: true,
		Output:  output,
	}, nil
}

// RequiresPermission returns true — external tools require permission by default.
func (t *BridgedTool) RequiresPermission() bool {
	return true
}

// IsRestricted returns false.
func (t *BridgedTool) IsRestricted() bool {
	return false
}

// ServerName returns the server name.
func (t *BridgedTool) ServerName() string {
	return t.serverName
}

// OriginalName returns the tool name without the server prefix.
func (t *BridgedTool) OriginalName() string {
	return t.mcpTool.Name
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
	mcpTools := session.Tools()
	bridged := make([]*BridgedTool, len(mcpTools))
	for i, tool := range mcpTools {
		bridged[i] = NewBridgedTool(session.Name(), tool, session)
	}
	return bridged
}

// mapJSONSchemaType converts a JSON Schema type to an Atmos ParamType.
func mapJSONSchemaType(jsonType string) tools.ParamType {
	switch jsonType {
	case "string":
		return tools.ParamTypeString
	case "integer", "number":
		return tools.ParamTypeInt
	case "boolean":
		return tools.ParamTypeBool
	case "array":
		return tools.ParamTypeArray
	case "object":
		return tools.ParamTypeObject
	default:
		return tools.ParamTypeString
	}
}
