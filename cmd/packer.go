package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// packerCmd represents the base command for all Packer sub-commands.
var packerCmd = &cobra.Command{
	Use:                "packer",
	Aliases:            []string{"pk"},
	Short:              "Manage packer-based machine images for multiple platforms",
	Long:               `This command runs Packer commands for creating identical machine images for multiple platforms from a single source configuration.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	packerCmd.DisableFlagParsing = true
	packerCmd.PersistentFlags().Bool("", false, doubleDashHint)
	AddStackCompletion(packerCmd)
	RootCmd.AddCommand(packerCmd)
}

func packerRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info := getConfigAndStacksInfo("packer", cmd, diffArgs)
	info.CliArgs = []string{"packer", commandName}
	err := e.ExecuteHelmfile(info)
	return err
}
