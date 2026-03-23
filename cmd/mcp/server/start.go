package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/mcp"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	transportStdio     = "stdio"
	transportHTTP      = "http"
	defaultHTTPPort    = 8080
	defaultHTTPHost    = "localhost"
	mcpProtocolVersion = "2025-03-26"
)

// transportConfig holds the validated transport configuration.
type transportConfig struct {
	transportType string
	host          string
	port          int
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Atmos MCP server",
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
        "args": ["mcp", "start"]
      }
    }
  }

Example usage with HTTP transport:
  atmos mcp start --transport http --port 8080

The server runs until interrupted (Ctrl+C) or the client disconnects.`,
	Example: `  # Start MCP server with stdio transport (default, for desktop clients)
  atmos mcp start

  # Start MCP server with HTTP transport
  atmos mcp start --transport http --port 8080

  # Start HTTP server on custom host and port
  atmos mcp start --transport http --host 127.0.0.1 --port 3000`,
	RunE: executeMCPServer,
}

func init() {
	// Add flags.
	startCmd.Flags().String("transport", transportStdio, "Transport type: stdio or http")
	startCmd.Flags().String("host", defaultHTTPHost, "Host to bind HTTP server (only for http transport)")
	startCmd.Flags().Int("port", defaultHTTPPort, "Port to bind HTTP server (only for http transport)")

	mcpcmd.McpCmd.AddCommand(startCmd)
}

func executeMCPServer(cmd *cobra.Command, args []string) error {
	defer ui.Info("MCP server stopped")

	// Get and validate transport flags.
	config, err := getTransportConfig(cmd)
	if err != nil {
		return err
	}

	// Setup MCP server.
	server, err := setupMCPServer()
	if err != nil {
		return err
	}

	// Create context with cancellation for signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server based on transport type.
	errChan := make(chan error, 1)

	switch config.transportType {
	case transportStdio:
		startStdioServer(ctx, server, errChan)
	case transportHTTP:
		startHTTPServer(server, config.host, config.port, errChan)
	default:
		return fmt.Errorf("%w: %s", errUtils.ErrMCPUnsupportedTransport, config.transportType)
	}

	// Wait for signal or error.
	return waitForShutdown(sigChan, errChan, cancel)
}

// getTransportConfig extracts and validates transport configuration from command flags.
func getTransportConfig(cmd *cobra.Command) (*transportConfig, error) {
	transportType, _ := cmd.Flags().GetString("transport")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")

	// Validate transport type.
	if transportType != transportStdio && transportType != transportHTTP {
		return nil, fmt.Errorf("%w: %s (must be 'stdio' or 'http')", errUtils.ErrMCPInvalidTransport, transportType)
	}

	return &transportConfig{
		transportType: transportType,
		host:          host,
		port:          port,
	}, nil
}

// setupMCPServer initializes the MCP server with all required components.
func setupMCPServer() (*mcp.Server, error) {
	// Load Atmos configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Check if MCP server is explicitly enabled.
	if !atmosConfig.MCP.Enabled {
		return nil, errUtils.ErrMCPNotEnabled
	}

	// Check if AI is enabled.
	if !atmosConfig.AI.Enabled {
		return nil, errUtils.ErrAINotEnabled
	}

	// Initialize tool registry and executor.
	registryRaw, executorRaw, err := initializeAIComponents(&atmosConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AI components: %w", err)
	}

	registry := registryRaw.(*tools.Registry)
	executor := executorRaw.(*tools.Executor)

	// Create MCP adapter and server.
	adapter := mcp.NewAdapter(registry, executor)
	return mcp.NewServer(adapter), nil
}

// waitForShutdown waits for either a shutdown signal or server error.
func waitForShutdown(sigChan chan os.Signal, errChan chan error, cancel context.CancelFunc) error {
	select {
	case sig := <-sigChan:
		ui.Infof("Received signal: %v", sig)
		cancel()
		return nil
	case err := <-errChan:
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("MCP server error: %w", err)
		}
		return nil
	}
}

// startStdioServer starts the MCP server with stdio transport.
func startStdioServer(ctx context.Context, server *mcp.Server, errChan chan error) {
	logServerInfo(server, transportStdio, "")
	go func() {
		transport := &mcpsdk.StdioTransport{}
		errChan <- server.Run(ctx, transport)
	}()
}

// startHTTPServer starts the MCP server with HTTP transport.
func startHTTPServer(server *mcp.Server, host string, port int, errChan chan error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	logServerInfo(server, transportHTTP, addr)
	go func() {
		handler := mcpsdk.NewSSEHandler(func(req *http.Request) *mcpsdk.Server {
			return server.SDK()
		}, nil)

		httpServer := &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
		}
		errChan <- httpServer.ListenAndServe()
	}()
}

// logServerInfo displays the server startup information to the user.
func logServerInfo(server *mcp.Server, transportType, addr string) {
	ui.Info("Starting Atmos MCP server...")
	serverInfo := server.ServerInfo()
	ui.Writef("  Server: %s v%s\n", serverInfo.Name, serverInfo.Version)
	ui.Writef("  Protocol: MCP %s\n", mcpProtocolVersion)
	if transportType == transportHTTP {
		ui.Writef("  Transport: HTTP (listening on %s)\n", addr)
		ui.Writef("    - SSE endpoint: http://%s/sse\n", addr)
		ui.Writef("    - Message endpoint: http://%s/message\n", addr)
	} else {
		ui.Writeln("  Transport: stdio")
	}
	ui.Info("Waiting for client connection...")
}

// initializeAIComponents initializes the AI tool registry and executor.
// This reuses the same initialization logic as the 'atmos ai chat' command.
func initializeAIComponents(atmosConfig *schema.AtmosConfiguration) (interface{}, interface{}, error) {
	// Import cmd package will give circular dependency, so we need to inline
	// the initialization here. This is the same pattern used in cmd/ai_chat.go

	if !atmosConfig.AI.Tools.Enabled {
		return nil, nil, errUtils.ErrAIToolsDisabled
	}

	log.Debug("Initializing AI tools")

	// Create tool registry.
	registry := tools.NewRegistry()

	// Register Atmos tools.
	if err := registry.Register(atmosTools.NewDescribeComponentTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register describe_component tool: %v", err)
	}
	if err := registry.Register(atmosTools.NewListStacksTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register list_stacks tool: %v", err)
	}
	if err := registry.Register(atmosTools.NewValidateStacksTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register validate_stacks tool: %v", err)
	}

	// Register file access tools (read/write for components and stacks).
	if err := registry.Register(atmosTools.NewReadComponentFileTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register read_component_file tool: %v", err)
	}
	if err := registry.Register(atmosTools.NewReadStackFileTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register read_stack_file tool: %v", err)
	}
	if err := registry.Register(atmosTools.NewWriteComponentFileTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register write_component_file tool: %v", err)
	}
	if err := registry.Register(atmosTools.NewWriteStackFileTool(atmosConfig)); err != nil {
		ui.Warningf("Failed to register write_stack_file tool: %v", err)
	}

	log.Debugf("Registered %d tools", registry.Count())

	// Create permission checker with MCP-appropriate settings.
	// For MCP server, use a non-interactive prompter since stdio is used for protocol.
	permConfig := &permission.Config{
		Mode:            getPermissionMode(atmosConfig),
		AllowedTools:    atmosConfig.AI.Tools.AllowedTools,
		RestrictedTools: atmosConfig.AI.Tools.RestrictedTools,
		BlockedTools:    atmosConfig.AI.Tools.BlockedTools,
		YOLOMode:        atmosConfig.AI.Tools.YOLOMode,
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
	if atmosConfig.AI.Tools.YOLOMode {
		return permission.ModeYOLO
	}

	if atmosConfig.AI.Tools.RequireConfirmation != nil && *atmosConfig.AI.Tools.RequireConfirmation {
		return permission.ModePrompt
	}

	return permission.ModeAllow
}
