package lsp

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/lsp/server"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewLSPCommand creates a new `atmos lsp` command.
func NewLSPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "lsp",
		Short:              "Language Server Protocol commands",
		Long:               "Start and manage the Atmos Language Server Protocol (LSP) server for IDE integration",
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	}

	cmd.AddCommand(NewLSPStartCommand())

	return cmd
}

// NewLSPStartCommand creates a new `atmos lsp start` command.
func NewLSPStartCommand() *cobra.Command {
	var transport string
	var address string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Atmos LSP server",
		Long: `Start the Atmos Language Server Protocol (LSP) server.

The LSP server enables IDE integration for Atmos stack files, providing:
- Syntax validation and diagnostics
- Auto-completion for Atmos keywords and components
- Hover documentation for Atmos-specific syntax
- Go to definition for stack references

The server can use different transports:
- stdio: Standard input/output (default, for IDE integration)
- tcp: TCP server (for remote connections)
- websocket: WebSocket server (for web-based editors)

Example usage with VS Code:
Configure your editor's LSP client to run:
  atmos lsp start --transport=stdio
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeLSPStart(cmd, transport, address)
		},
	}

	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport protocol: stdio, tcp, or websocket")
	cmd.Flags().StringVar(&address, "address", "localhost:7777", "Address for tcp/websocket transports (host:port)")

	return cmd
}

// executeLSPStart runs the LSP server.
func executeLSPStart(_ *cobra.Command, transport, address string) error {
	ctx := context.Background()

	// Load Atmos configuration.
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Create LSP server.
	lspServer, err := server.NewServer(ctx, &atmosConfig)
	if err != nil {
		return err
	}

	// Log server start.
	log.Info("Starting Atmos LSP server...")
	log.Info(fmt.Sprintf("Transport: %s", transport))
	if transport != "stdio" {
		log.Info(fmt.Sprintf("Address: %s", address))
	}

	// Run server based on transport type.
	switch transport {
	case "stdio":
		return lspServer.RunStdio()

	case "tcp":
		log.Info(fmt.Sprintf("Listening on TCP %s", address))
		return lspServer.RunTCP(address)

	case "websocket", "ws":
		log.Info(fmt.Sprintf("Listening on WebSocket %s", address))
		return lspServer.RunWebSocket(address)

	default:
		return fmt.Errorf("%w: %s (must be 'stdio', 'tcp', or 'websocket')", errUtils.ErrLSPInvalidTransport, transport)
	}
}
