package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version"
)

// Server wraps the official MCP SDK server with Atmos-specific functionality.
type Server struct {
	sdk      *mcpsdk.Server
	adapter  *Adapter
	registry *tools.Registry
}

// NewServer creates a new MCP server using the official SDK.
func NewServer(adapter *Adapter) *Server {
	defer perf.Track(nil, "mcp.NewServer")()

	// Create SDK server with Atmos implementation details.
	sdk := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "atmos-mcp-server",
		Version: version.Version,
	}, nil)

	s := &Server{
		sdk:      sdk,
		adapter:  adapter,
		registry: adapter.registry,
	}

	// Register all Atmos tools with the SDK.
	s.registerTools()

	return s
}

// registerTools registers all Atmos tools with the MCP SDK server.
func (s *Server) registerTools() {
	toolsList := s.registry.List()

	for _, tool := range toolsList {
		toolName := tool.Name()
		log.Debug(fmt.Sprintf("Registering MCP tool: %s", toolName))

		// Create a closure to capture the tool name for the handler.
		toolNameCopy := toolName

		// Create handler function using SDK's CallToolRequest type.
		handler := func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			// Unmarshal arguments from raw JSON.
			var args map[string]interface{}
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
					return &mcpsdk.CallToolResult{
						Content: []mcpsdk.Content{
							&mcpsdk.TextContent{Text: fmt.Sprintf("Failed to parse arguments: %v", err)},
						},
						IsError: true,
					}, nil
				}
			}

			return s.handleToolCall(ctx, toolNameCopy, args)
		}

		// Add tool to SDK server with input schema.
		s.sdk.AddTool(&mcpsdk.Tool{
			Name:        toolName,
			Description: tool.Description(),
			InputSchema: s.generateInputSchema(tool),
		}, handler)
	}

	log.Debug(fmt.Sprintf("Registered %d MCP tools", len(toolsList)))
}

// handleToolCall executes a tool and returns the result in SDK format.
func (s *Server) handleToolCall(ctx context.Context, name string, arguments map[string]interface{}) (*mcpsdk.CallToolResult, error) {
	log.Info(fmt.Sprintf("Executing tool: %s", name))

	// Execute the tool using our adapter.
	result, err := s.adapter.ExecuteTool(ctx, name, arguments)
	if err != nil {
		// Return error as tool result.
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{
				&mcpsdk.TextContent{Text: fmt.Sprintf("Tool execution failed: %v", err)},
			},
			IsError: true,
		}, nil
	}

	// Convert our result to SDK format.
	content := make([]mcpsdk.Content, 0)

	// Add output as text content.
	if result.Output != "" {
		content = append(content, &mcpsdk.TextContent{Text: result.Output})
	}

	// Add error if present.
	if result.Error != nil {
		content = append(content, &mcpsdk.TextContent{Text: fmt.Sprintf("Error: %v", result.Error)})
	}

	// Add data if present.
	if len(result.Data) > 0 {
		for key, value := range result.Data {
			content = append(content, &mcpsdk.TextContent{
				Text: fmt.Sprintf("Data '%s': %v", key, value),
			})
		}
	}

	return &mcpsdk.CallToolResult{
		Content: content,
		IsError: !result.Success,
	}, nil
}

// SDK returns the underlying SDK server instance.
func (s *Server) SDK() *mcpsdk.Server {
	defer perf.Track(nil, "mcp.Server.SDK")()

	return s.sdk
}

// ServerInfo returns the server implementation information.
func (s *Server) ServerInfo() mcpsdk.Implementation {
	defer perf.Track(nil, "mcp.Server.ServerInfo")()

	return mcpsdk.Implementation{
		Name:    "atmos-mcp-server",
		Version: version.Version,
	}
}

// generateInputSchema converts tool parameters to JSON Schema format for the SDK.
func (s *Server) generateInputSchema(tool tools.Tool) map[string]interface{} {
	params := tool.Parameters()
	if len(params) == 0 {
		// Return minimal schema for tools with no parameters.
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": make(map[string]interface{}),
	}

	properties := schema["properties"].(map[string]interface{})
	required := make([]string, 0)

	for _, param := range params {
		propSchema := map[string]interface{}{
			"description": param.Description,
			"type":        mapParamTypeToJSONSchema(param.Type),
		}

		if param.Default != nil {
			propSchema["default"] = param.Default
		}

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

// mapParamTypeToJSONSchema maps tool parameter types to JSON Schema types.
func mapParamTypeToJSONSchema(paramType tools.ParamType) string {
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

// Run starts the server with the given transport.
func (s *Server) Run(ctx context.Context, transport mcpsdk.Transport) error {
	defer perf.Track(nil, "mcp.Server.Run")()

	log.Info("MCP server started (using official SDK)")
	return s.sdk.Run(ctx, transport)
}
