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
	fullArgs := []string{commandName}
	if parsedConfig.ComponentName != "" {
		fullArgs = append(fullArgs, parsedConfig.ComponentName)
	}
	fullArgs = append(fullArgs, parsedConfig.PassThroughArgs...)

	info := getConfigAndStacksInfo("helmfile", cmd, fullArgs)
	info.CliArgs = []string{"helmfile", commandName}
	err = e.ExecuteHelmfile(info)
	return err
}
