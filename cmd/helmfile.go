package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
)

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Manage Helmfile-based Kubernetes deployments",
	Long:               `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Args:               cobra.NoArgs,
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().Bool("", false, doubleDashHint)
	AddStackCompletion(helmfileCmd)
	RootCmd.AddCommand(helmfileCmd)
}

func helmfileRun(cmd *cobra.Command, commandName string, args []string) error {
	handleHelpRequest(cmd, args)
	// Enable heatmap tracking if --heatmap flag is present in os.Args
	// (needed because flag parsing is disabled for helmfile commands).
	enableHeatmapIfRequested()
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info, err := getConfigAndStacksInfo("helmfile", cmd, diffArgs)
	if err != nil {
		return err
	}
	info.CliArgs = []string{"helmfile", commandName}
	err = e.ExecuteHelmfile(info)
	return err
}
