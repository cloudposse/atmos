package client

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

var toolsCmd = &cobra.Command{
	Use:   "tools <name>",
	Short: "List tools from an MCP server",
	Long:  "Connect to an external MCP server and list its available tools.",
	Args:  cobra.ExactArgs(1),
	RunE:  executeMCPTools,
}

func init() {
	mcpcmd.McpCmd.AddCommand(toolsCmd)
}

func executeMCPTools(cmd *cobra.Command, args []string) error {
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

	if err := mgr.Start(ctx, name); err != nil {
		return err
	}

	session, err := mgr.Get(name)
	if err != nil {
		return err
	}

	tools := session.Tools()
	if len(tools) == 0 {
		fmt.Fprintln(os.Stdout, "No tools available from "+name)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TOOL\tDESCRIPTION")

	const maxDescLen = 80

	for _, tool := range tools {
		desc := tool.Description
		if len(desc) > maxDescLen {
			desc = desc[:maxDescLen-3] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\n", tool.Name, desc)
	}

	return w.Flush()
}
