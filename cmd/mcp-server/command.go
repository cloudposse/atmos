package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/mcp"
	"github.com/cloudposse/atmos/pkg/mcp/protocol"
	"github.com/cloudposse/atmos/pkg/mcp/transport"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	transportStdio     = "stdio"
	transportHTTP      = "http"
	defaultHTTPPort    = 8080
	defaultHTTPHost    = "localhost"
	mcpProtocolVersion = "2025-03-26"
)

// NewCommand creates a new mcp-server command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp-server",
		Short: "Start Atmos MCP (Model Context Protocol) server",
		Long: `Start an MCP server that exposes Atmos AI tools via the Model Context Protocol.

The MCP server allows AI assistants (Claude Desktop, Claude Code, VSCode, etc.) to access
Atmos infrastructure management capabilities through a standardized protocol.

The server supports two transport modes:
- stdio: Standard input/output for local desktop applications (default)
- http: HTTP with Server-Sent Events (SSE) for remote/cloud clients

Example usage with Claude Desktop (stdio):
  Add to ~/.config/claude/claude_desktop_config.json:
  {
    "mcpServers": {
      "atmos": {
        "command": "atmos",
        "args": ["mcp-server"]
      }
    }
  }

Example usage with HTTP transport:
  atmos mcp-server --transport http --port 8080

The server runs until interrupted (Ctrl+C) or the client disconnects.`,
		Example: `  # Start MCP server with stdio transport (default, for desktop clients)
  atmos mcp-server

  # Start MCP server with HTTP transport
  atmos mcp-server --transport http --port 8080

  # Start HTTP server on custom host and port
  atmos mcp-server --transport http --host 0.0.0.0 --port 3000`,
		RunE: executeMCPServer,
	}

	// Add flags.
	cmd.Flags().String("transport", transportStdio, "Transport type: stdio or http")
	cmd.Flags().String("host", defaultHTTPHost, "Host to bind HTTP server (only for http transport)")
	cmd.Flags().Int("port", defaultHTTPPort, "Port to bind HTTP server (only for http transport)")

	return cmd
}

func executeMCPServer(cmd *cobra.Command, args []string) error {
	defer log.Info("MCP server stopped")

	// Get transport flags.
	transportType, _ := cmd.Flags().GetString("transport")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")

	// Validate transport type.
	if transportType != transportStdio && transportType != transportHTTP {
		return fmt.Errorf("%w: %s (must be 'stdio' or 'http')", errUtils.ErrMCPInvalidTransport, transportType)
	}

	// Load Atmos configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Check if AI is enabled.
	if !atmosConfig.Settings.AI.Enabled {
		return errUtils.ErrAINotEnabled
	}

	// Initialize tool registry and executor.
	registryRaw, executorRaw, err := initializeAIComponents(&atmosConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize AI components: %w", err)
	}

	registry := registryRaw.(*tools.Registry)
	executor := executorRaw.(*tools.Executor)

	// Create MCP adapter and server.
	adapter := mcp.NewAdapter(registry, executor)
	server := mcp.NewServer(adapter)

	// Select and run transport.
	switch transportType {
	case transportStdio:
		logServerInfo(server, transportStdio, "")
		stdioTransport := transport.NewStdioTransport()
		return runServerWithSignalHandling(stdioTransport, server)

	case transportHTTP:
		addr := fmt.Sprintf("%s:%d", host, port)
		logServerInfo(server, transportHTTP, addr)
		httpTransport := transport.NewHTTPTransport(addr)
		return runServerWithSignalHandling(httpTransport, server)

	default:
		return fmt.Errorf("%w: %s", errUtils.ErrMCPUnsupportedTransport, transportType)
	}
}

// logServerInfo logs the server startup information.
func logServerInfo(server *mcp.Server, transportType, addr string) {
	log.Info("Starting Atmos MCP server...")
	log.Info(fmt.Sprintf("Server: %s v%s", server.ServerInfo().Name, server.ServerInfo().Version))
	log.Info(fmt.Sprintf("Protocol: MCP %s", mcpProtocolVersion))
	if transportType == transportHTTP {
		log.Info(fmt.Sprintf("Transport: HTTP (listening on %s)", addr))
		log.Info(fmt.Sprintf("  - SSE endpoint: http://%s/sse", addr))
		log.Info(fmt.Sprintf("  - Message endpoint: http://%s/message", addr))
		log.Info(fmt.Sprintf("  - Health endpoint: http://%s/health", addr))
	} else {
		log.Info("Transport: stdio")
	}
	log.Info("Waiting for client connection...")
}

// serverTransport defines the interface for MCP transports.
type serverTransport interface {
	Serve(ctx context.Context, handler protocol.Handler) error
}

// runServerWithSignalHandling runs the server with signal handling.
func runServerWithSignalHandling(trans serverTransport, server *mcp.Server) error {
	// Set up context with cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine.
	errChan := make(chan error, 1)
	go func() {
		errChan <- trans.Serve(ctx, server.Handler())
	}()

	// Wait for signal or error.
	select {
	case sig := <-sigChan:
		log.Info(fmt.Sprintf("Received signal: %v", sig))
		cancel()
		return nil
	case err := <-errChan:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("MCP server error: %w", err)
		}
		return nil
	}
}

// initializeAIComponents initializes the AI tool registry and executor.
// This reuses the same initialization logic as the 'atmos ai chat' command.
func initializeAIComponents(atmosConfig *schema.AtmosConfiguration) (interface{}, interface{}, error) {
	// Import cmd package will give circular dependency, so we need to inline
	// the initialization here. This is the same pattern used in cmd/ai_chat.go

	if !atmosConfig.Settings.AI.Tools.Enabled {
		return nil, nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing AI tools")

	// Create tool registry.
	registry := tools.NewRegistry()

	// Register Atmos tools.
	if err := registry.Register(atmosTools.NewDescribeComponentTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register describe_component tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewListStacksTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register list_stacks tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewValidateStacksTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register validate_stacks tool: %v", err))
	}

	// Register file access tools (read/write for components and stacks).
	if err := registry.Register(atmosTools.NewReadComponentFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register read_component_file tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewReadStackFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register read_stack_file tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewWriteComponentFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register write_component_file tool: %v", err))
	}
	if err := registry.Register(atmosTools.NewWriteStackFileTool(atmosConfig)); err != nil {
		log.Warn(fmt.Sprintf("Failed to register write_stack_file tool: %v", err))
	}

	log.Debug(fmt.Sprintf("Registered %d tools", registry.Count()))

	// Create permission checker with MCP-appropriate settings.
	// For MCP server, use a non-interactive prompter since stdio is used for protocol.
	permConfig := &permission.Config{
		Mode:            getPermissionMode(atmosConfig),
		AllowedTools:    atmosConfig.Settings.AI.Tools.AllowedTools,
		RestrictedTools: atmosConfig.Settings.AI.Tools.RestrictedTools,
		BlockedTools:    atmosConfig.Settings.AI.Tools.BlockedTools,
		YOLOMode:        atmosConfig.Settings.AI.Tools.YOLOMode,
	}
	// Use YOLO mode for MCP to avoid blocking on prompts (client handles permissions).
	permConfig.YOLOMode = true
	permChecker := permission.NewChecker(permConfig, permission.NewCLIPrompter())

	// Create tool executor.
	executor := tools.NewExecutor(registry, permChecker, tools.DefaultTimeout)
	log.Debug("Tool executor initialized")

	return registry, executor, nil
}

// getPermissionMode determines the permission mode from config.
func getPermissionMode(atmosConfig *schema.AtmosConfiguration) permission.Mode {
	if atmosConfig.Settings.AI.Tools.YOLOMode {
		return permission.ModeYOLO
	}

	if atmosConfig.Settings.AI.Tools.RequireConfirmation {
		return permission.ModePrompt
	}

	return permission.ModeAllow
}
