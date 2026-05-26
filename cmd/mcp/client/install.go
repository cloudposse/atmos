package client

import (
	_ "embed"
	"errors"
	"fmt"
	"sort"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	mcpinstall "github.com/cloudposse/atmos/pkg/mcp/install"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_install.md
var installLongMarkdown string

var installCmd = &cobra.Command{
	Use:   "install [server-name...]",
	Short: "Install configured MCP servers into AI client config files",
	Long:  installLongMarkdown,
	Args:  cobra.ArbitraryArgs,
	RunE:  executeMCPInstall,
}

var installParser *flags.StandardParser

const installScopeFlag = "scope"

var (
	errMCPServerNotConfigured = errors.New("MCP server is not configured")
	errNoMCPClientsSelected   = errors.New("no MCP clients selected")
)

func init() {
	installParser = flags.NewStandardParser(
		flags.WithStringSliceFlag("client", "c", nil, "MCP client to install into (repeatable): claude-code, cursor, vscode, codex, gemini"),
		flags.WithEnvVars("client", "ATMOS_MCP_CLIENT"),
		flags.WithBoolFlag("all-clients", "", false, "Install into all supported MCP clients"),
		flags.WithEnvVars("all-clients", "ATMOS_MCP_ALL_CLIENTS"),
		flags.WithStringFlag(installScopeFlag, "", mcpinstall.ScopeProject, "Install scope: project or user"),
		flags.WithEnvVars(installScopeFlag, "ATMOS_MCP_SCOPE"),
		flags.WithValidValues(installScopeFlag, mcpinstall.ScopeProject, mcpinstall.ScopeUser),
		flags.WithBoolFlag("global", "g", false, "Alias for --scope user"),
		flags.WithEnvVars("global", "ATMOS_MCP_GLOBAL"),
		flags.WithBoolFlag("yes", "y", false, "Skip confirmation prompts"),
		flags.WithEnvVars("yes", "ATMOS_YES"),
		flags.WithBoolFlag("dry-run", "", false, "Show what would be installed without writing files"),
		flags.WithEnvVars("dry-run", "ATMOS_DRY_RUN"),
		flags.WithBoolFlag("force", "", false, "Overwrite existing server entries without prompting"),
		flags.WithEnvVars("force", "ATMOS_FORCE"),
		flags.WithBoolFlag("gitignore", "", false, "Add generated project config files to .gitignore"),
		flags.WithEnvVars("gitignore", "ATMOS_MCP_GITIGNORE"),
	)
	installParser.RegisterFlags(installCmd)
	if err := installParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	mcpcmd.McpCmd.AddCommand(installCmd)
}

func executeMCPInstall(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.mcpInstall")()
	v := viper.GetViper()
	if err := installParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}
	servers, err := selectServers(atmosConfig.MCP.Servers, args)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		ui.Info("No MCP servers configured. Add servers under `mcp.servers` in `atmos.yaml`.")
		return nil
	}

	scope := resolveInstallScope(cmd, v)
	clients, err := resolveInstallClients(&atmosConfig, scope, v)
	if err != nil {
		return err
	}

	return installServers(&atmosConfig, servers, installCommandOptions{
		clients:    clients,
		scope:      scope,
		allClients: v.GetBool("all-clients"),
		yes:        v.GetBool("yes"),
		force:      v.GetBool("force"),
		dryRun:     v.GetBool("dry-run"),
		gitignore:  v.GetBool("gitignore"),
	})
}

type installCommandOptions struct {
	clients    []string
	scope      string
	allClients bool
	yes        bool
	force      bool
	dryRun     bool
	gitignore  bool
}

func selectServers(configured map[string]schema.MCPServerConfig, names []string) (map[string]schema.MCPServerConfig, error) {
	if len(names) == 0 {
		return configured, nil
	}
	selected := make(map[string]schema.MCPServerConfig, len(names))
	for _, name := range names {
		server, ok := configured[name]
		if !ok {
			return nil, fmt.Errorf("%w: %q", errMCPServerNotConfigured, name)
		}
		selected[name] = server
	}
	return selected, nil
}

func resolveInstallScope(cmd *cobra.Command, v *viper.Viper) string {
	if cmd != nil && cmd.Flags().Changed(installScopeFlag) {
		return v.GetString(installScopeFlag)
	}
	if v.GetBool("global") {
		return mcpinstall.ScopeUser
	}
	return v.GetString(installScopeFlag)
}

func resolveInstallClients(atmosConfig *schema.AtmosConfiguration, scope string, v *viper.Viper) ([]string, error) {
	clients := v.GetStringSlice("client")
	if len(clients) > 0 || v.GetBool("all-clients") {
		return clients, nil
	}

	basePath := installBasePath(atmosConfig)
	detected := mcpinstall.DetectClients(basePath, "", scope)
	if v.GetBool("yes") {
		if len(detected) > 0 {
			return detected, nil
		}
		return append([]string(nil), mcpinstall.SupportedClients...), nil
	}
	if term.IsTTYSupportForStdin() && !telemetry.IsCI() {
		return promptForMCPClients(detected)
	}
	if len(detected) > 0 {
		return detected, nil
	}
	return nil, errUtils.ErrInteractiveNotAvailable
}

func promptForMCPClients(defaultClients []string) ([]string, error) {
	selected := append([]string(nil), defaultClients...)
	if len(selected) == 0 {
		selected = append([]string(nil), mcpinstall.SupportedClients...)
	}
	selectedByClient := make(map[string]bool, len(selected))
	for _, client := range selected {
		selectedByClient[client] = true
	}
	options := make([]huh.Option[string], 0, len(mcpinstall.SupportedClients))
	for _, client := range mcpinstall.SupportedClients {
		options = append(options, huh.NewOption(client, client).Selected(selectedByClient[client]))
	}
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("ctrl+c/esc", "cancel"),
	)
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Install MCP servers into which clients?").
				Description("Space toggles, enter confirms.").
				Options(options...).
				Value(&selected),
		),
	).WithKeyMap(keyMap).WithTheme(uiutils.NewAtmosHuhTheme())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errUtils.ErrUserAborted
		}
		return nil, err
	}
	if len(selected) == 0 {
		return nil, errNoMCPClientsSelected
	}
	sort.Strings(selected)
	return selected, nil
}

func installServers(
	atmosConfig *schema.AtmosConfiguration,
	servers map[string]schema.MCPServerConfig,
	opts installCommandOptions,
) error {
	basePath := installBasePath(atmosConfig)
	installer, err := mcpinstall.New(
		mcpinstall.WithBasePath(basePath),
		mcpinstall.WithScope(opts.scope),
		mcpinstall.WithClients(opts.clients),
		mcpinstall.WithAllClients(opts.allClients),
		mcpinstall.WithOverwrite(opts.force),
		mcpinstall.WithDryRun(opts.dryRun),
		mcpinstall.WithGitignore(opts.gitignore),
		mcpinstall.WithToolchainPath(buildToolchainPATH(atmosConfig)),
		mcpinstall.WithOnConflict(mcpConflictHandler(opts.yes)),
	)
	if err != nil {
		return err
	}
	result, err := installer.Install(servers)
	if err != nil {
		return err
	}
	reportMCPInstallResult(result, opts.dryRun)
	return nil
}

func installBasePath(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig.BasePath != "" {
		return atmosConfig.BasePath
	}
	return "."
}

func mcpConflictHandler(yes bool) mcpinstall.ConflictFunc {
	if yes {
		return func(mcpinstall.Target, string) (bool, error) {
			return false, nil
		}
	}
	return func(target mcpinstall.Target, serverName string) (bool, error) {
		return flags.PromptForConfirmation(
			fmt.Sprintf("Overwrite MCP server %q in %s?", serverName, target.Path),
			false,
		)
	}
}

func reportMCPInstallResult(result *mcpinstall.Result, dryRun bool) {
	prefixCreated := "Created"
	prefixUpdated := "Updated"
	if dryRun {
		prefixCreated = "Would create"
		prefixUpdated = "Would update"
	}
	for _, path := range result.CreatedFiles {
		ui.Successf("%s `%s`", prefixCreated, path)
	}
	for _, path := range result.UpdatedFiles {
		ui.Successf("%s `%s`", prefixUpdated, path)
	}
	for _, skipped := range result.SkippedServers {
		ui.Warningf("Skipped `%s`", skipped)
	}
	for _, path := range result.GitignoredFiles {
		ui.Successf("Added `%s` to .gitignore", path)
	}
}
