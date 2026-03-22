package mcp

import (
	"context"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var testCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Test connectivity to an MCP integration",
	Long:  "Start an external MCP server, verify the initialization handshake, list tools, and ping the server.",
	Args:  cobra.ExactArgs(1),
	RunE:  executeMCPTest,
}

func init() {
	mcpCmd.AddCommand(testCmd)
}

func executeMCPTest(cmd *cobra.Command, args []string) error {
	name := args[0]

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	mgr, err := mcpclient.NewManager(atmosConfig.MCP.Integrations)
	if err != nil {
		return err
	}
	defer mgr.StopAll() //nolint:errcheck // Best-effort cleanup.

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	result := mgr.Test(ctx, name)
	printTestResult(result)
	return result.Error
}

// printTestResult displays the test results with success/failure indicators.
func printTestResult(result *mcpclient.TestResult) {
	if result.ServerStarted {
		ui.Success("Server started successfully")
	} else {
		ui.Error("Server failed to start")
	}

	if result.Initialized {
		ui.Success("Initialization handshake complete")
	}

	if result.ToolCount > 0 {
		ui.Successf("%d tools available", result.ToolCount)
	} else if result.ServerStarted {
		ui.Warning("No tools available")
	}

	if result.PingOK {
		ui.Success("Server responds to ping")
	} else if result.ServerStarted {
		ui.Warning("Server did not respond to ping")
	}
}
