package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/mcp/protocol"
	"github.com/cloudposse/atmos/pkg/version"
)

// Server implements an MCP server that exposes Atmos AI tools.
type Server struct {
	adapter     *Adapter
	handler     *protocol.DefaultHandler
	initialized bool
	serverInfo  protocol.Implementation
}

// NewServer creates a new MCP server.
func NewServer(adapter *Adapter) *Server {
	s := &Server{
		adapter:     adapter,
		handler:     protocol.NewDefaultHandler(),
		initialized: false,
		serverInfo: protocol.Implementation{
			Name:    "atmos-mcp-server",
			Version: version.Version,
		},
	}

	// Register method handlers.
	s.registerHandlers()

	return s
}

// registerHandlers registers all MCP protocol handlers.
func (s *Server) registerHandlers() {
	s.handler.RegisterMethod(protocol.MethodInitialize, s.handleInitialize)
	s.handler.RegisterMethod(protocol.MethodPing, s.handlePing)
	s.handler.RegisterMethod(protocol.MethodToolsList, s.handleToolsList)
	s.handler.RegisterMethod(protocol.MethodToolsCall, s.handleToolsCall)

	// Register notifications.
	s.handler.RegisterNotification(protocol.MethodInitialized, s.handleInitialized)
}

// handleInitialize handles the initialize request.
func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var initParams protocol.InitializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &initParams); err != nil {
			return nil, &protocol.Error{
				Code:    protocol.ErrorCodeInvalidParams,
				Message: "invalid initialize params",
				Data:    err.Error(),
			}
		}
	}

	log.Info(fmt.Sprintf("MCP client connected: %s v%s",
		initParams.ClientInfo.Name,
		initParams.ClientInfo.Version))

	// Return server capabilities.
	result := protocol.InitializeResult{
		ProtocolVersion: protocol.ProtocolVersion,
		ServerInfo:      s.serverInfo,
		Capabilities: protocol.ServerCapabilities{
			Tools: &protocol.ToolsCapability{
				ListChanged: false,
			},
		},
		Instructions: "Atmos MCP Server provides access to Atmos infrastructure management tools. " +
			"Use the available tools to list components, describe stacks, validate configurations, and plan Terraform changes.",
	}

	return result, nil
}

// handleInitialized handles the initialized notification.
func (s *Server) handleInitialized(ctx context.Context, params json.RawMessage) error {
	s.initialized = true
	log.Info("MCP server initialized")
	return nil
}

// handlePing handles ping requests.
func (s *Server) handlePing(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return map[string]interface{}{"status": "pong"}, nil
}

// handleToolsList handles tools/list requests.
func (s *Server) handleToolsList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if !s.initialized {
		return nil, &protocol.Error{
			Code:    protocol.ErrorCodeInvalidRequest,
			Message: "server not initialized",
		}
	}

	tools, err := s.adapter.ListTools(ctx)
	if err != nil {
		return nil, &protocol.Error{
			Code:    protocol.ErrorCodeInternalError,
			Message: "failed to list tools",
			Data:    err.Error(),
		}
	}

	return protocol.ToolsListResult{
		Tools: tools,
	}, nil
}

// handleToolsCall handles tools/call requests.
func (s *Server) handleToolsCall(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if !s.initialized {
		return nil, &protocol.Error{
			Code:    protocol.ErrorCodeInvalidRequest,
			Message: "server not initialized",
		}
	}

	var callParams protocol.CallToolParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, &protocol.Error{
			Code:    protocol.ErrorCodeInvalidParams,
			Message: "invalid call tool params",
			Data:    err.Error(),
		}
	}

	log.Info(fmt.Sprintf("Executing tool: %s", callParams.Name))

	result, err := s.adapter.ExecuteTool(ctx, callParams.Name, callParams.Arguments)
	if err != nil {
		return nil, &protocol.Error{
			Code:    protocol.ErrorCodeInternalError,
			Message: "tool execution failed",
			Data:    err.Error(),
		}
	}

	return result, nil
}

// Handler returns the protocol handler.
func (s *Server) Handler() protocol.Handler {
	return s.handler
}

// ServerInfo returns the server implementation information.
func (s *Server) ServerInfo() protocol.Implementation {
	return s.serverInfo
}
