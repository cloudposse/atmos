package client

import (
	_ "embed"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	mcpinstall "github.com/cloudposse/atmos/pkg/mcp/install"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_uninstall.md
var uninstallLongMarkdown string

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [server-name...]",
	Short: "Remove installed MCP servers from AI client config files",
	Long:  uninstallLongMarkdown,
	Args:  cobra.ArbitraryArgs,
	RunE:  executeMCPUninstall,
}

var uninstallParser *flags.StandardParser

func init() {
	uninstallParser = flags.NewStandardParser(
		flags.WithStringSliceFlag("client", "c", nil, "MCP client to uninstall from (repeatable): "+strings.Join(mcpinstall.SupportedClients, ", ")),
		flags.WithEnvVars("client", "ATMOS_MCP_CLIENT"),
		flags.WithBoolFlag("all-clients", "", false, "Uninstall from all supported MCP clients"),
		flags.WithEnvVars("all-clients", "ATMOS_MCP_ALL_CLIENTS"),
		flags.WithStringFlag(installScopeFlag, "", mcpinstall.ScopeProject, "Uninstall scope: project or user"),
		flags.WithEnvVars(installScopeFlag, "ATMOS_MCP_SCOPE"),
		flags.WithValidValues(installScopeFlag, mcpinstall.ScopeProject, mcpinstall.ScopeUser),
		flags.WithBoolFlag("global", "g", false, "Alias for --scope user"),
		flags.WithEnvVars("global", "ATMOS_MCP_GLOBAL"),
		flags.WithBoolFlag(yesFlag, "y", false, "Skip confirmation prompts"),
		flags.WithEnvVars(yesFlag, "ATMOS_YES"),
		flags.WithBoolFlag(dryRunFlag, "", false, "Show what would be removed without writing files"),
		flags.WithEnvVars(dryRunFlag, "ATMOS_DRY_RUN"),
	)
	uninstallParser.RegisterFlags(uninstallCmd)
	if err := uninstallParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	mcpcmd.McpCmd.AddCommand(uninstallCmd)
}

func executeMCPUninstall(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.mcpUninstall")()
	v := viper.GetViper()
	if err := uninstallParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	names := args
	if len(names) == 0 {
		names = sortedServerNames(atmosConfig.MCP.Servers)
	}
	if len(names) == 0 {
		ui.Info(noServersConfiguredMessage(atmosConfig.MCP.Enabled))
		return nil
	}

	scope := resolveInstallScope(cmd, v)
	clients, err := resolveInstallClients(&atmosConfig, scope, v)
	if err != nil {
		return err
	}
	if len(clients) == 0 {
		ui.Warningf("No AI clients detected to uninstall from — run `atmos mcp uninstall --client <client>` (or `--all-clients`) to uninstall manually.")
		return nil
	}

	basePath := installBasePath(&atmosConfig)
	installer, err := mcpinstall.New(
		mcpinstall.WithBasePath(basePath),
		mcpinstall.WithScope(scope),
		mcpinstall.WithClients(clients),
		mcpinstall.WithAllClients(v.GetBool("all-clients")),
		mcpinstall.WithDryRun(v.GetBool(dryRunFlag)),
	)
	if err != nil {
		return err
	}
	result, err := installer.Uninstall(names)
	if err != nil {
		return err
	}
	reportMCPUninstallResult(result, v.GetBool(dryRunFlag))
	return nil
}

func sortedServerNames(servers map[string]schema.MCPServerConfig) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func reportMCPUninstallResult(result *mcpinstall.Result, dryRun bool) {
	prefixRemoved := "Removed"
	if dryRun {
		prefixRemoved = "Would remove"
	}
	for _, removed := range result.RemovedServers {
		ui.Successf("%s `%s`", prefixRemoved, removed)
	}
	for _, notFound := range result.NotFoundServers {
		ui.Warningf("Not installed: `%s`", notFound)
	}
	if !dryRun {
		for _, path := range result.UpdatedFiles {
			ui.Successf("Updated `%s`", path)
		}
	}
}
