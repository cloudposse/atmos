package server

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
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
	"github.com/cloudposse/atmos/pkg/signals"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	transportStdio      = "stdio"
	transportHTTP       = "http"
	defaultHTTPPort     = 8080
	defaultHTTPHost     = "localhost"
	mcpProtocolVersion  = "2025-03-26"
	shutdownGracePeriod = 250 * time.Millisecond
)

// transportConfig holds the validated transport configuration.
type transportConfig struct {
	transportType string
	host          string
	port          int
}

//go:embed markdown/atmos_mcp_start.md
var startLongMarkdown string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Atmos MCP server",
	Long:  startLongMarkdown,
	Args:  cobra.NoArgs,
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
	)
	startParser.RegisterFlags(startCmd)
	if err := startParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	mcpcmd.McpCmd.AddCommand(startCmd)
}

func executeMCPServer(cmd *cobra.Command, args []string) error {
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

	releaseInterruptExit := signals.SuspendInterruptExit()
	defer releaseInterruptExit()

	// Create context with cancellation for signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

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
	return waitForShutdownWithStopMessage(sigChan, errChan, cancel, func() {
		ui.Info("MCP server stopped")
	})
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
	// Load base Atmos configuration. Stack graph tools load stack manifests
	// lazily so the MCP server can start before stacks exist or while stack
	// imports are temporarily broken.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Check if MCP server is explicitly enabled.
	if !atmosConfig.MCP.Enabled {
		return nil, errUtils.Build(errUtils.ErrMCPNotEnabled).
			WithHint("Run `atmos config set mcp.enabled true`, or add to atmos.yaml:\n\n```yaml\nmcp:\n  enabled: true\n```").
			Err()
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
	return waitForShutdownWithStopMessage(sigChan, errChan, cancel, nil)
}

func waitForShutdownWithStopMessage(sigChan chan os.Signal, errChan chan error, cancel context.CancelFunc, onStop func()) error {
	select {
	case <-sigChan:
		if onStop != nil {
			onStop()
		}
		cancel()
		return waitForServerStop(errChan)
	case err := <-errChan:
		if onStop != nil {
			onStop()
		}
		return normalizeServerStopError(err)
	}
}

func waitForServerStop(errChan chan error) error {
	select {
	case err, ok := <-errChan:
		if !ok {
			return nil
		}
		return normalizeServerStopError(err)
	case <-time.After(shutdownGracePeriod):
		return nil
	}
}

func normalizeServerStopError(err error) error {
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("MCP server error: %w", err)
	}
	return nil
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
	ui.Success("MCP running. Waiting for an agent…")
}

// initializeAIComponents initializes the AI tool registry and executor.
// This reuses the same initialization logic as the 'atmos ai chat' command
// by calling the canonical atmosTools.RegisterTools factory in
// pkg/ai/tools/atmos/setup.go. Previously this function hand-rolled
// a curated subset of seven tools, which meant docs (e.g.
// website/docs/ai/mcp-server.mdx) and the actual exposed set drifted
// (notably: docs advertised `describe_affected` but it was never
// registered here). Delegating to the shared factory eliminates the
// drift and exposes the full Atmos AI tool surface to MCP clients.
//
// The nil passed for the LSP manager is intentional — MCP servers do
// not have an LSP context. RegisterTools handles this gracefully (the
// LSP-only `validate_file_lsp` tool is skipped when lspManager is nil).
func initializeAIComponents(atmosConfig *schema.AtmosConfiguration) (interface{}, interface{}, error) {
	registry := tools.NewRegistry()

	// `ai.tools.enabled` governs whether `atmos ai chat`/`ask`/`exec` — Atmos's own AI
	// assistant — is allowed to call tools during its reasoning loop. It doesn't apply here:
	// the MCP server's entire purpose is exposing Atmos tools to external clients, and
	// `mcp.enabled: true` (already required to reach this function) is its own, sufficient
	// opt-in. So tools are always registered for MCP, regardless of ai.tools.enabled.
	log.Debug("Initializing AI tools")

	// atmos_list_commands/atmos_command_help snapshot the Cobra command tree lazily via this
	// injected provider, since pkg/ai/tools/atmos cannot import cmd/internal directly (Go's
	// internal-package visibility rule restricts it to importers rooted at cmd/). This is the
	// one place in the cmd/ tree wired to do that lookup.
	atmosTools.SetCommandTreeProvider(func() []*atmosTools.CommandNode {
		var roots []*atmosTools.CommandNode
		for group, providers := range internal.ListProviders() {
			for _, provider := range providers {
				if cmd := provider.GetCommand(); cmd != nil {
					roots = append(roots, atmosTools.NewCommandNodeFromCobra(cmd, group))
				}
			}
		}
		return roots
	})

	// Delegate to the shared registration factory. Errors are surfaced as
	// warnings rather than fatals to preserve the previous hand-rolled
	// behavior — a single tool failing to register should not prevent the
	// MCP server from starting with the rest.
	if err := atmosTools.RegisterTools(registry, atmosConfig, nil); err != nil {
		ui.Warningf("Failed to register one or more AI tools: %v", err)
	}

	log.Debugf("Registered %d tools", registry.Count())

	// Create permission checker with MCP-appropriate settings. Allowed/Restricted/Blocked
	// still apply — they govern *which* tools and *how* they run, not whether tool-serving
	// is enabled at all.
	// For MCP server, use a non-interactive prompter since stdio is used for protocol.
	permConfig := &permission.Config{
		Mode:       getPermissionMode(atmosConfig),
		Allowed:    atmosConfig.AI.Tools.Allowed,
		Restricted: atmosConfig.AI.Tools.Restricted,
		Blocked:    atmosConfig.AI.Tools.Blocked,
		YOLOMode:   atmosConfig.AI.Tools.YOLOMode,
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
