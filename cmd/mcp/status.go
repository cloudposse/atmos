package mcp

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all MCP integrations",
	Long:  "Start all configured MCP integrations and display their connection status, tool counts, and health.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		defer perf.Track(nil, "cmd.mcpStatus")()

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		if len(atmosConfig.MCP.Integrations) == 0 {
			fmt.Fprintln(os.Stdout, "No MCP integrations configured.")
			return nil
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

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tSTATUS\tTOOLS\tDESCRIPTION")

		for _, session := range mgr.List() {
			result := mgr.Test(ctx, session.Name())
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

			errSuffix := ""
			if result.Error != nil {
				errSuffix = fmt.Sprintf(" (%s)", result.Error)
				const maxErrLen = 50
				if len(errSuffix) > maxErrLen {
					errSuffix = errSuffix[:47] + "...)"
				}
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s%s\n",
				session.Name(),
				status,
				toolCount,
				session.Config().Description,
				errSuffix,
			)
		}

		return w.Flush()
	},
}

func init() {
	mcpCmd.AddCommand(statusCmd)
}
