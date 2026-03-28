package client

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

//go:embed markdown/atmos_mcp_tools.md
var toolsLongMarkdown string

var toolsCmd = &cobra.Command{
	Use:   "tools <name>",
	Short: "List tools from an MCP server",
	Long:  toolsLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE:  executeMCPTools,
}

func init() {
	mcpcmd.McpCmd.AddCommand(toolsCmd)
}

func executeMCPTools(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.mcpTools")()
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

	startOpts := buildStartOptions(&atmosConfig)
	if err := mgr.Start(ctx, name, startOpts...); err != nil {
		return err
	}

	session, err := mgr.Get(name)
	if err != nil {
		return err
	}

	tools := session.Tools()
	if len(tools) == 0 {
		ui.Info("No tools available from " + name)
		return nil
	}

	headers := []string{"TOOL", "DESCRIPTION"}
	var rows [][]string
	for _, tool := range tools {
		// Use only the first sentence for table display.
		desc := firstSentence(tool.Description)
		rows = append(rows, []string{tool.Name, desc})
	}

	fmt.Fprintln(os.Stderr, theme.CreateMinimalTable(headers, rows))
	return nil
}

// firstSentence extracts the first sentence from a description, collapsing whitespace.
// Returns the complete first sentence (up to period+space), or truncates at a markdown
// header boundary if no sentence break is found.
func firstSentence(desc string) string {
	// Collapse all whitespace (newlines, tabs, multiple spaces) into single spaces.
	desc = strings.Join(strings.Fields(desc), " ")

	// Stop at first period followed by a space (end of sentence).
	if idx := strings.Index(desc, ". "); idx > 0 {
		return desc[:idx+1]
	}

	// Stop at markdown header.
	if idx := strings.Index(desc, " ##"); idx > 0 {
		return strings.TrimSpace(desc[:idx]) + "."
	}

	return desc
}
