package cmd

import (
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

// helmfileCmd represents the base command for all helmfile sub-commands
var helmfileCmd = &cobra.Command{
	Use:                "helmfile",
	Aliases:            []string{"hf"},
	Short:              "Execute 'helmfile' commands",
	Long:               `This command runs Helmfile commands`,
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
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}

		// Check for the latest Atmos release on GitHub and print update message
		latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
		if err == nil && latestReleaseTag != "" {
			latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
			currentRelease := strings.TrimPrefix(version.Version, "v")
			if latestRelease != currentRelease {
				u.PrintMessageToUpgradeToAtmosLatestRelease(latestRelease)
			}
		}
		// Exit on help
		if info.NeedHelp {
			return
		}
		// Check Atmos configuration
		checkAtmosConfig()

		err = e.ExecuteHelmfile(info)
		if err != nil {
			u.LogErrorAndExit(schema.CliConfiguration{}, err)
		}
	},
}

func init() {
	// https://github.com/spf13/cobra/issues/739
	helmfileCmd.DisableFlagParsing = true
	helmfileCmd.PersistentFlags().StringP("stack", "s", "", "atmos helmfile <helmfile_command> <component> -s <stack>")
	RootCmd.AddCommand(helmfileCmd)
}
