package client

import (
	_ "embed"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_list.md
var listLongMarkdown string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured MCP servers",
	Long:  listLongMarkdown,
	RunE:  executeMCPList,
}

func init() {
	mcpcmd.McpCmd.AddCommand(listCmd)
}

func executeMCPList(_ *cobra.Command, _ []string) error {
	defer perf.Track(nil, "cmd.mcpList")()
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	if len(atmosConfig.MCP.Servers) == 0 {
		ui.Info("No MCP servers configured. Add servers under 'mcp.servers' in atmos.yaml.")
		return nil
	}

	mgr, err := mcpclient.NewManager(atmosConfig.MCP.Servers)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tDESCRIPTION")

	for _, session := range mgr.List() {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			session.Name(),
			session.Status(),
			session.Config().Description,
		)
	}

	return w.Flush()
}
