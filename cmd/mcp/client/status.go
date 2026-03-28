package client

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_mcp_status.md
var statusLongMarkdown string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all MCP servers",
	Long:  statusLongMarkdown,
	RunE: func(cmd *cobra.Command, _ []string) error {
		defer perf.Track(nil, "cmd.mcpStatus")()

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		if len(atmosConfig.MCP.Servers) == 0 {
			ui.Info("No MCP servers configured.")
			return nil
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

		startOpts := buildStartOptions(&atmosConfig)
		headers := []string{"NAME", "STATUS", "TOOLS", "DESCRIPTION"}
		var rows [][]string

		for _, session := range mgr.List() {
			result := mgr.Test(ctx, session.Name(), startOpts...)
			status := "error"
			toolCount := "0"

			if result.PingOK {
				status = "running"
			} else if result.ServerStarted {
				status = "degraded"
			}

			if result.ToolCount > 0 {
				toolCount = fmt.Sprintf("%d", result.ToolCount)
			}

			desc := session.Config().Description
			if result.Error != nil {
				const maxErrLen = 50
				errMsg := result.Error.Error()
				if len(errMsg) > maxErrLen {
					errMsg = errMsg[:maxErrLen-3] + "..."
				}
				desc += " (" + errMsg + ")"
			}

			rows = append(rows, []string{session.Name(), status, toolCount, desc})
		}

		fmt.Fprintln(os.Stderr, theme.CreateMinimalTable(headers, rows))
		return nil
	},
}

func init() {
	mcpcmd.McpCmd.AddCommand(statusCmd)
}
