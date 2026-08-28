package client

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	mcpconfig "github.com/cloudposse/atmos/pkg/mcp/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_remove.md
var removeLongMarkdown string

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an MCP server from mcp.servers in atmos.yaml",
	Long:  removeLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE:  executeMCPRemove,
}

var removeParser *flags.StandardParser

func init() {
	removeParser = flags.NewStandardParser(
		flags.WithBoolFlag(yesFlag, "y", false, "Skip confirmation prompts"),
		flags.WithEnvVars(yesFlag, "ATMOS_YES"),
	)
	removeParser.RegisterFlags(removeCmd)
	if err := removeParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	mcpcmd.McpCmd.AddCommand(removeCmd)
}

func executeMCPRemove(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.mcpRemove")()
	v := viper.GetViper()
	if err := removeParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	name := args[0]
	file, err := mcpconfig.ResolveFile(cmd, &atmosConfig)
	if err != nil {
		return err
	}

	exists, err := mcpconfig.Exists(file, name)
	if err != nil {
		return err
	}
	if !exists {
		return errUtils.Build(errUtils.ErrMCPServerNotFound).
			WithExplanation(fmt.Sprintf("%q is not configured under mcp.servers in %s", name, file)).
			WithHint("Run `atmos mcp list` to see configured servers.").
			Err()
	}

	if !v.GetBool(yesFlag) {
		confirmed, err := flags.PromptForConfirmation(
			fmt.Sprintf("Remove MCP server %q from %s?", name, file),
			false,
		)
		if err != nil {
			return err
		}
		if !confirmed {
			ui.Warningf("Skipped removing `%s`", name)
			return nil
		}
	}

	if err := mcpconfig.Remove(file, name); err != nil {
		return err
	}
	ui.Successf("Removed `%s` from `mcp.servers` in %s", name, file)
	ui.Infof("If `%s` was installed into an AI client, run `atmos mcp uninstall %s` to remove it there too.", name, name)
	return nil
}
