package mcp

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured MCP integrations",
	Long:  "List all external MCP server integrations configured in atmos.yaml.",
	RunE:  executeMCPList,
}

func init() {
	mcpCmd.AddCommand(listCmd)
}

func executeMCPList(_ *cobra.Command, _ []string) error {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	if len(atmosConfig.MCP.Integrations) == 0 {
		fmt.Fprintln(os.Stdout, "No MCP integrations configured. Add integrations under 'mcp.integrations' in atmos.yaml.")
		return nil
	}

	mgr, err := mcpclient.NewManager(atmosConfig.MCP.Integrations)
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
