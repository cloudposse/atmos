package mcp

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/mcp/protocol"
)

const (
	// JsonSchemaTypeKey is the JSON Schema property name for type.
	JsonSchemaTypeKey = "type"
)

// Adapter adapts existing Atmos AI tools to MCP protocol.
type Adapter struct {
	registry *tools.Registry
	executor *tools.Executor
}

// NewAdapter creates a new tool adapter.
func NewAdapter(registry *tools.Registry, executor *tools.Executor) *Adapter {
	return &Adapter{
		registry: registry,
		executor: executor,
	}
}

// ListTools returns all available tools in MCP format.
func (a *Adapter) ListTools(ctx context.Context) ([]protocol.Tool, error) {
	toolsList := a.registry.List()
	mcpTools := make([]protocol.Tool, 0, len(toolsList))

	for _, tool := range toolsList {
		mcpTool := protocol.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: a.parametersToSchema(tool.Parameters()),
		}

		mcpTools = append(mcpTools, mcpTool)
	}

	return mcpTools, nil
}

// ExecuteTool executes a tool and returns the result in MCP format.
func (a *Adapter) ExecuteTool(ctx context.Context, name string, arguments map[string]interface{}) (*protocol.CallToolResult, error) {
	// Execute the tool using the existing executor.
	result, err := a.executor.Execute(ctx, name, arguments)
	if err != nil {
		// Return error as MCP content.
		return &protocol.CallToolResult{
			Content: []protocol.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Tool execution failed: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	// Convert result to MCP content.
	return a.convertResultToMCP(result), nil
}

// convertResultToMCP converts a tool result to MCP format.
func (a *Adapter) convertResultToMCP(result *tools.Result) *protocol.CallToolResult {
	mcpResult := &protocol.CallToolResult{
		Content: make([]protocol.Content, 0),
		IsError: !result.Success,
	}

	// Add output as text content.
	if result.Output != "" {
		mcpResult.Content = append(mcpResult.Content, protocol.Content{
			Type: "text",
			Text: result.Output,
		})
	}

	// Add error if present.
	if result.Error != nil {
		mcpResult.Content = append(mcpResult.Content, protocol.Content{
			Type: "text",
			Text: fmt.Sprintf("Error: %v", result.Error),
		})
		mcpResult.IsError = true
	}

	// Add data if present.
	if len(result.Data) > 0 {
		for key, value := range result.Data {
			mcpResult.Content = append(mcpResult.Content, protocol.Content{
				Type: "text",
				Text: fmt.Sprintf("Data '%s': %v", key, value),
			})
		}
	}

	return mcpResult
}

// GetTool retrieves a specific tool by name.
func (a *Adapter) GetTool(name string) (protocol.Tool, error) {
	tool, err := a.registry.Get(name)
	if err != nil {
		return protocol.Tool{}, fmt.Errorf("%w: %s", errUtils.ErrMCPToolNotFound, name)
	}

	return protocol.Tool{
		Name:        tool.Name(),
		Description: tool.Description(),
		InputSchema: a.parametersToSchema(tool.Parameters()),
	}, nil
}

// parametersToSchema converts tool parameters to JSON Schema format.
func (a *Adapter) parametersToSchema(params []tools.Parameter) map[string]interface{} {
	schema := map[string]interface{}{
		JsonSchemaTypeKey: "object",
		"properties":      make(map[string]interface{}),
	}

	properties := schema["properties"].(map[string]interface{})
	required := make([]string, 0)

	for _, param := range params {
		propSchema := a.buildPropertySchema(param)
		properties[param.Name] = propSchema

		if param.Required {
			required = append(required, param.Name)
		}
	}

	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// buildPropertySchema builds a JSON Schema property from a tool parameter.
func (a *Adapter) buildPropertySchema(param tools.Parameter) map[string]interface{} {
	propSchema := map[string]interface{}{
		"description": param.Description,
	}

	// Map tool parameter types to JSON Schema types.
	propSchema[JsonSchemaTypeKey] = mapParamTypeToJSONSchemaType(param.Type)

	if param.Default != nil {
		propSchema["default"] = param.Default
	}

	return propSchema
}

// mapParamTypeToJSONSchemaType maps tool parameter types to JSON Schema types.
func mapParamTypeToJSONSchemaType(paramType tools.ParamType) string {
	switch paramType {
	case tools.ParamTypeString:
		return "string"
	case tools.ParamTypeInt:
		return "integer"
	case tools.ParamTypeBool:
		return "boolean"
	case tools.ParamTypeArray:
		return "array"
	case tools.ParamTypeObject:
		return "object"
	default:
		return "string"
	}
}
