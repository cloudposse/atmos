package client

import (
	"context"
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_test.md
var testLongMarkdown string

var testCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Test connectivity to an MCP server",
	Long:  testLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE:  executeMCPTest,
}

func init() {
	mcpcmd.McpCmd.AddCommand(testCmd)
}

func executeMCPTest(cmd *cobra.Command, args []string) error {
	name := args[0]

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	mgr, err := mcpclient.NewManager(atmosConfig.MCP.Servers)
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
