package pro

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/browser"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	mcpinstall "github.com/cloudposse/atmos/pkg/mcp/install"
	"github.com/cloudposse/atmos/pkg/pro/install"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_pro_install.md
var installLongMarkdown string

//go:embed markdown/atmos_pro_install_next_steps.md
var nextStepsMarkdown string

// installCmd scaffolds Atmos Pro configuration files.
var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Atmos Pro workflows and configuration",
	Long:  installLongMarkdown,
	Args:  cobra.NoArgs,
	RunE:  runInstall,
}

var installParser *flags.StandardParser

const (
	proInstallMCPFlag        = "mcp"
	proInstallClientFlag     = "client"
	proInstallAllClientsFlag = "all-clients"
	proInstallScopeFlag      = "scope"
	proInstallGlobalFlag     = "global"
	proInstallGitignoreFlag  = "gitignore"
)

var (
	errNoMCPClientsSelected = errors.New("no MCP clients selected")
	errMCPOnlyFlag          = errors.New("flag can only be used with --mcp")
)

func init() {
	installParser = flags.NewStandardParser(
		flags.WithBoolFlag("yes", "y", false, "Skip confirmation prompts"),
		flags.WithEnvVars("yes", "ATMOS_YES"),
		flags.WithBoolFlag("force", "", false, "Force operation without confirmation"),
		flags.WithEnvVars("force", "ATMOS_FORCE"),
		flags.WithBoolFlag("dry-run", "", false, "Simulate operation without making changes"),
		flags.WithEnvVars("dry-run", "ATMOS_DRY_RUN"),
		flags.WithBoolFlag(proInstallMCPFlag, "", false, "Install the Atmos Pro MCP server only"),
		flags.WithEnvVars(proInstallMCPFlag, "ATMOS_PRO_INSTALL_MCP"),
		flags.WithStringSliceFlag(proInstallClientFlag, "c", nil, "MCP client to install into (repeatable): claude-code, cursor, vscode, codex, gemini"),
		flags.WithEnvVars(proInstallClientFlag, "ATMOS_MCP_CLIENT"),
		flags.WithBoolFlag(proInstallAllClientsFlag, "", false, "Install into all supported MCP clients"),
		flags.WithEnvVars(proInstallAllClientsFlag, "ATMOS_MCP_ALL_CLIENTS"),
		flags.WithStringFlag(proInstallScopeFlag, "", mcpinstall.ScopeProject, "Install scope for --mcp: project or user"),
		flags.WithEnvVars(proInstallScopeFlag, "ATMOS_MCP_SCOPE"),
		flags.WithValidValues(proInstallScopeFlag, mcpinstall.ScopeProject, mcpinstall.ScopeUser),
		flags.WithBoolFlag(proInstallGlobalFlag, "g", false, "Alias for --scope user when used with --mcp"),
		flags.WithEnvVars(proInstallGlobalFlag, "ATMOS_MCP_GLOBAL"),
		flags.WithBoolFlag(proInstallGitignoreFlag, "", false, "Add generated project MCP config files to .gitignore"),
		flags.WithEnvVars(proInstallGitignoreFlag, "ATMOS_MCP_GITIGNORE"),
	)

	installParser.RegisterFlags(installCmd)

	if err := installParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

// resolveFromGlobalFlags parses global flags and resolves install paths.
func resolveFromGlobalFlags(cmd *cobra.Command, v *viper.Viper) (basePath, stacksBasePath string) {
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	info := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}

	return resolveInstallPaths(&info)
}

// resolveInstallPaths loads atmos config and resolves base/stacks paths.
func resolveInstallPaths(info *schema.ConfigAndStacksInfo) (basePath, stacksBasePath string) {
	atmosConfig, err := cfg.InitCliConfig(*info, false)
	if err != nil {
		ui.Warning("Could not load atmos config, using default paths")
		return ".", "stacks"
	}
	basePath = atmosConfig.BasePath
	if basePath == "" {
		basePath = "."
	}
	stacksBasePath = atmosConfig.Stacks.BasePath
	if stacksBasePath == "" {
		stacksBasePath = "stacks"
	}
	return basePath, stacksBasePath
}

func runInstall(cmd *cobra.Command, _ []string) error {
	v := viper.GetViper()
	if err := installParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	yes := v.GetBool("yes")
	force := v.GetBool("force")
	dryRun := v.GetBool("dry-run")
	if v.GetBool(proInstallMCPFlag) {
		return runInstallMCP(cmd, v, yes, force, dryRun)
	}
	if err := validateMCPOnlyInstallFlags(cmd); err != nil {
		return err
	}

	basePath, stacksBasePath := resolveFromGlobalFlags(cmd, v)
	return runStandardInstall(basePath, stacksBasePath, yes, force, dryRun)
}

func runStandardInstall(basePath, stacksBasePath string, yes, force, dryRun bool) error {
	// Prompt for confirmation unless --yes or --dry-run.
	if !dryRun && !yes {
		confirmed, err := flags.PromptForConfirmation(
			"Install Atmos Pro workflows and configuration?",
			false,
		)
		if err != nil {
			return fmt.Errorf("confirmation failed: %w", err)
		}
		if !confirmed {
			ui.Warning("Installation cancelled")
			return nil
		}
	}

	opts := buildInstallOpts(basePath, stacksBasePath, force, yes)
	installer := install.NewInstaller(&install.OSFileWriter{}, opts...)

	if dryRun {
		reportDryRun(installer.DryRun())
		return nil
	}

	result, err := installer.Install()
	if err != nil {
		return err
	}

	reportResult(result)
	ui.Writeln("")
	ui.MarkdownMessage(nextStepsMarkdown)

	if !yes {
		promptOpenWorkspace()
	}

	return nil
}

const atmosProMCPURL = "https://atmos-pro.com/mcp"

func runInstallMCP(cmd *cobra.Command, v *viper.Viper, yes, force, dryRun bool) error {
	basePath, _ := resolveFromGlobalFlags(cmd, v)
	scope := v.GetString(proInstallScopeFlag)
	if !cmd.Flags().Changed(proInstallScopeFlag) && v.GetBool(proInstallGlobalFlag) {
		scope = mcpinstall.ScopeUser
	}
	clients := v.GetStringSlice(proInstallClientFlag)
	allClients := v.GetBool(proInstallAllClientsFlag)
	if len(clients) == 0 && !allClients {
		detected := mcpinstall.DetectClients(basePath, "", scope)
		switch {
		case len(detected) > 0:
			clients = detected
		case yes:
			allClients = true
		default:
			return fmt.Errorf("%w: pass --client or --all-clients", errNoMCPClientsSelected)
		}
	}

	installer, err := mcpinstall.New(
		mcpinstall.WithBasePath(basePath),
		mcpinstall.WithScope(scope),
		mcpinstall.WithClients(clients),
		mcpinstall.WithAllClients(allClients),
		mcpinstall.WithOverwrite(force),
		mcpinstall.WithDryRun(dryRun),
		mcpinstall.WithGitignore(v.GetBool(proInstallGitignoreFlag)),
		mcpinstall.WithOnConflict(proMCPConflictHandler(yes)),
	)
	if err != nil {
		return err
	}

	result, err := installer.Install(map[string]schema.MCPServerConfig{
		"atmos-pro": {
			Type:        schema.MCPTransportHTTP,
			URL:         atmosProMCPURL,
			Description: "Atmos Pro drift, deployments, workflow runs, audit log, Repairs, and setup panels",
		},
	})
	if err != nil {
		return err
	}
	reportMCPResult(result, dryRun)
	return nil
}

func validateMCPOnlyInstallFlags(cmd *cobra.Command) error {
	for _, name := range []string{proInstallClientFlag, proInstallAllClientsFlag, proInstallScopeFlag, proInstallGlobalFlag, proInstallGitignoreFlag} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("%w: --%s", errMCPOnlyFlag, name)
		}
	}
	return nil
}

func proMCPConflictHandler(yes bool) mcpinstall.ConflictFunc {
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

func reportMCPResult(result *mcpinstall.Result, dryRun bool) {
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

// buildInstallOpts constructs the installer options based on flags.
func buildInstallOpts(basePath, stacksBasePath string, force, yes bool) []install.Option {
	opts := []install.Option{
		install.WithBasePath(basePath),
		install.WithStacksBasePath(stacksBasePath),
		install.WithForce(force),
	}
	if !force {
		if yes {
			// --yes skips all prompts; silently skip existing files.
			opts = append(opts, install.WithOnConflict(func(string) (bool, error) {
				return false, nil
			}))
		} else {
			opts = append(opts, install.WithOnConflict(promptOverwrite))
		}
	}
	return opts
}

// promptOverwrite prompts the user to overwrite an existing file.
// In interactive TTY mode, shows a confirmation prompt.
// In non-TTY mode, returns an error.
func promptOverwrite(relPath string) (bool, error) {
	return flags.PromptForConfirmation(
		fmt.Sprintf("Overwrite %s?", relPath),
		false,
	)
}

const workspaceURL = "https://atmos-pro.com/onboarding/create-workspace"

// promptOpenWorkspace asks the user if they want to open the workspace creation page.
func promptOpenWorkspace() {
	confirmed, err := flags.PromptForConfirmation(
		"Open Atmos Pro workspace setup in your browser?",
		false,
	)
	if err != nil || !confirmed {
		return
	}

	if err := browser.New().Open(workspaceURL); err != nil {
		ui.Warningf("Could not open browser: %s", err)
		ui.Infof("Visit: %s", workspaceURL)
	}
}

// reportResult displays the installation results.
func reportResult(result *install.InstallResult) {
	for _, f := range result.CreatedFiles {
		ui.Successf("Created `%s`", f)
	}
	for _, f := range result.UpdatedFiles {
		ui.Successf("Updated `%s`", f)
	}
	for _, f := range result.SkippedFiles {
		ui.Warningf("Skipped `%s` (already exists, use --force to overwrite)", f)
	}
}

// reportDryRun displays what would happen during installation.
func reportDryRun(result *install.InstallResult) {
	ui.Infof("Dry run - no files will be written\n")
	for _, f := range result.CreatedFiles {
		ui.Infof("Would create `%s`", f)
	}
	for _, f := range result.UpdatedFiles {
		ui.Infof("Would update `%s`", f)
	}
	for _, f := range result.SkippedFiles {
		ui.Warningf("Would skip `%s` (already exists)", f)
	}
}
