package client

import (
	_ "embed"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_mcp_list.md
var listLongMarkdown string

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured MCP servers",
	Long:  listLongMarkdown,
	Args:  cobra.NoArgs,
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

	headers := []string{"NAME", "STATUS", "DESCRIPTION"}
	var rows [][]string
	for _, session := range mgr.List() {
		rows = append(rows, []string{
			session.Name(),
			string(session.Status()),
			session.Config().Description,
		})
	}

	ui.Writeln(theme.CreateMinimalTable(headers, rows))
	return nil
}
