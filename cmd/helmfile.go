package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flagparser"
)

// helmfileParser handles flag parsing for helmfile commands.
var helmfileParser *flagparser.PassThroughFlagParser

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:     "helmfile",
	Aliases: []string{"hf"},
	Short:   "Manage Helmfile-based Kubernetes deployments",
	Long:    `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	Args:    cobra.NoArgs,
}

func init() {
	// Create parser with Helmfile flags.
	// This replaces DisableFlagParsing and manual flag handling.
	helmfileParser = flagparser.NewPassThroughFlagParser(
		flagparser.WithHelmfileFlags(),
	)

	// Helmfile passes subcommand separately to helmfileRun, so only extract 1 positional arg (component).
	helmfileParser.SetPositionalArgsCount(1)

	// Register flags with Cobra.
	helmfileParser.RegisterFlags(helmfileCmd)
	_ = helmfileParser.BindToViper(viper.GetViper())

	AddStackCompletion(helmfileCmd)
	RootCmd.AddCommand(helmfileCmd)
}

func helmfileRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)

	// Parse args with flagparser
	ctx := cmd.Context()
	parsedConfig, err := helmfileParser.Parse(ctx, args)
	if err != nil {
		return err
	}

	// Build args array from ParsedConfig for getConfigAndStacksInfo
	// PositionalArgs contains [component] for helmfile commands
	fullArgs := append([]string{commandName}, parsedConfig.PositionalArgs...)
	fullArgs = append(fullArgs, parsedConfig.PassThroughArgs...)

	info := getConfigAndStacksInfo("helmfile", cmd, fullArgs)

	// Override info fields with values from parsedConfig.AtmosFlags to respect precedence (CLI > ENV > defaults).
	// parsedConfig.AtmosFlags contains values resolved by Viper with proper precedence handling.
	if val, ok := parsedConfig.AtmosFlags["stack"]; ok {
		info.Stack = val.(string)
	}
	if val, ok := parsedConfig.AtmosFlags["identity"]; ok {
		info.Identity = val.(string)
	}
	if val, ok := parsedConfig.AtmosFlags["dry-run"]; ok {
		info.DryRun = val.(bool)
	}

	// Handle --identity flag for interactive selection.
	// If identity is "__SELECT__", prompt for interactive selection.
	if info.Identity == IdentityFlagSelectValue {
		handleInteractiveIdentitySelection(&info)
	}

	info.CliArgs = []string{"helmfile", commandName}
	err = e.ExecuteHelmfile(info)
	return err
}
