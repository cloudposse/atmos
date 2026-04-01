package server

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	atmosTools "github.com/cloudposse/atmos/pkg/ai/tools/atmos"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
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
	apiKey        string
}

//go:embed markdown/atmos_mcp_start.md
var startLongMarkdown string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Atmos MCP server",
	Long:  startLongMarkdown,
	Example: `  # Start MCP server with stdio transport (default, for desktop clients)
  atmos mcp start

  # Start MCP server with HTTP transport
  atmos mcp start --transport http --port 8080

  # Start HTTP server on custom host and port
  atmos mcp start --transport http --host 127.0.0.1 --port 3000`,
	RunE: executeMCPServer,
}

var startParser *flags.StandardParser

func init() {
	startParser = flags.NewStandardParser(
		flags.WithStringFlag("transport", "", transportStdio, "Transport type: stdio or http"),
		flags.WithStringFlag("host", "", defaultHTTPHost, "Host to bind HTTP server (only for http transport)"),
		flags.WithIntFlag("port", "", defaultHTTPPort, "Port to bind HTTP server (only for http transport)"),
		flags.WithStringFlag("api-key", "", "", "Bearer token for HTTP transport authentication (also read from ATMOS_MCP_API_KEY; required when --host is a non-loopback address)"),
	)
	startParser.RegisterFlags(startCmd)
	if err := startParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

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
		startHTTPServer(server, config.host, config.port, config.apiKey, errChan)
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
	apiKey, _ := cmd.Flags().GetString("api-key")

	// Fall back to ATMOS_MCP_API_KEY environment variable when the flag is not set.
	if apiKey == "" {
		apiKey = os.Getenv("ATMOS_MCP_API_KEY")
	}

	// Validate transport type.
	if transportType != transportStdio && transportType != transportHTTP {
		return nil, fmt.Errorf("%w: %s (must be 'stdio' or 'http')", errUtils.ErrMCPInvalidTransport, transportType)
	}

	// For HTTP transport, enforce an API key when binding to a non-loopback address.
	if transportType == transportHTTP && !isLoopbackHost(host) && apiKey == "" {
		return nil, fmt.Errorf("%w: --api-key (or ATMOS_MCP_API_KEY) is required when --host is a non-loopback address", errUtils.ErrMCPHTTPAuthRequired)
	}

	return &transportConfig{
		transportType: transportType,
		host:          host,
		port:          port,
		apiKey:        apiKey,
	}, nil
}

// isLoopbackHost reports whether host resolves to a loopback address only.
// It accepts "localhost", IPv4 loopback (127.x.x.x), and IPv6 loopback (::1).
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	logServerInfo(server, transportStdio, "", false, false)
	go func() {
		transport := &mcpsdk.StdioTransport{}
		errChan <- server.Run(ctx, transport)
	}()
}

// startHTTPServer starts the MCP server with HTTP transport.
func startHTTPServer(server *mcp.Server, host string, port int, apiKey string, errChan chan error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	logServerInfo(server, transportHTTP, addr, apiKey != "", isLoopbackHost(host))
	go func() {
		var handler http.Handler = mcpsdk.NewSSEHandler(func(req *http.Request) *mcpsdk.Server {
			return server.SDK()
		}, nil)

		if apiKey != "" {
			handler = bearerTokenMiddleware(apiKey, handler)
		}

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

// bearerTokenMiddleware enforces Bearer token authentication on all requests.
// It uses constant-time comparison to prevent timing-based token enumeration.
func bearerTokenMiddleware(apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		provided := authHeader[len(prefix):]
		if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// logServerInfo displays the server startup information to the user.
func logServerInfo(server *mcp.Server, transportType, addr string, authenticated, loopback bool) {
	ui.Info("Starting Atmos MCP server...")
	serverInfo := server.ServerInfo()
	ui.Writef("  Server: %s v%s\n", serverInfo.Name, serverInfo.Version)
	ui.Writef("  Protocol: MCP %s\n", mcpProtocolVersion)
	if transportType == transportHTTP {
		ui.Writef("  Transport: HTTP (listening on %s)\n", addr)
		ui.Writef("    - SSE endpoint: http://%s/sse\n", addr)
		ui.Writef("    - Message endpoint: http://%s/message\n", addr)
		if authenticated {
			ui.Info("  Authentication: Bearer token required")
		} else if loopback {
			ui.Warning("  Authentication: NONE — endpoints are unauthenticated; use --api-key for production deployments")
		} else {
			ui.Warning("  Authentication: NONE — endpoints are network-accessible without authentication")
		}
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
