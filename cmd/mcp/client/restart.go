package client

import (
	"context"
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_restart.md
var restartLongMarkdown string

var restartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart an MCP server",
	Long:  restartLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.mcpRestart")()

		name := args[0]

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		mgr, err := mcpclient.NewManager(atmosConfig.MCP.Servers)
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		// Stop if running (ignore error — it may not be running).
		_ = mgr.Stop(name)

		// Start fresh with toolchain resolution.
		startOpts := buildStartOptions(&atmosConfig)
		if err := mgr.Start(ctx, name, startOpts...); err != nil {
			return err
		}

		session, err := mgr.Get(name)
		if err != nil {
			return err
		}

		ui.Successf("Restarted MCP server %q (%d tools available)", name, len(session.Tools()))

		// Keep running until context is cancelled (for interactive use).
		// For non-interactive use (CI), the process will exit after this.
		_ = mgr.Stop(name)
		return nil
	},
}

func init() {
	mcpcmd.McpCmd.AddCommand(restartCmd)
}
