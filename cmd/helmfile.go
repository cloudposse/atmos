package cmd

import (
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Manage Helmfile-based Kubernetes deployments",
	Long:               `This command runs Helmfile commands to manage Kubernetes deployments using Helmfile.`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	Run: func(cmd *cobra.Command, args []string) {

		var argsAfterDoubleDash []string
		var finalArgs = args

		doubleDashIndex := lo.IndexOf(args, "--")
		if doubleDashIndex > 0 {
			finalArgs = lo.Slice(args, 0, doubleDashIndex)
			argsAfterDoubleDash = lo.Slice(args, doubleDashIndex+1, len(args))
		}

		info, err := e.ProcessCommandLineArgs("helmfile", cmd, finalArgs, argsAfterDoubleDash)
		if err != nil {
			u.LogErrorAndExit(err)
		}
		// Exit on help
		if info.NeedHelp || (info.SubCommand == "" && info.SubCommand2 == "") {
			// Check for the latest Atmos release on GitHub and print update message
			if err := cmd.Help(); err != nil {
				u.LogErrorAndExit(err)
			}
			return
		}
		// Check Atmos configuration
		checkAtmosConfig()

		err = e.ExecuteHelmfile(info)
		if err != nil {
			u.LogErrorAndExit(err)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile <helmfile_command> <component> -s <stack>")
	RootCmd.AddCommand(helmfileCmd)
}
