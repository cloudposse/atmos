package cmd

import (
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/telemetry"
	u "github.com/cloudposse/atmos/pkg/utils"
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

func helmfileRun(cmd *cobra.Command, commandName string, args []string) {
	handleHelpRequest(cmd, args)
	diffArgs := []string{commandName}
	diffArgs = append(diffArgs, args...)
	info := getConfigAndStacksInfo("helmfile", cmd, diffArgs)
	info.CliArgs = []string{"helmfile", commandName}
	err := e.ExecuteHelmfile(info)
	if err != nil {
		telemetry.CaptureCmd(cmd, err)
		u.PrintErrorMarkdownAndExit("", err, "")
	}
	telemetry.CaptureCmd(cmd)
}
