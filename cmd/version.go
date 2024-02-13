package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var Version = "0.0.1"

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Print the CLI version",
	Long:    `This command prints the CLI version`,
	Example: "atmos version",
	Run: func(cmd *cobra.Command, args []string) {
		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(err)
		}

		u.PrintMessage(fmt.Sprintf("\U0001F47D Atmos %s on %s/%s", Version, runtime.GOOS, runtime.GOARCH))
		fmt.Println()

		// Check for the latest Atmos release on GitHub
		latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
		if err == nil && latestReleaseTag != "" {
			latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
			currentRelease := strings.TrimPrefix(Version, "v")
			if latestRelease != currentRelease {
				printMessageToUpgradeToAtmosLatestRelease(latestRelease)
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
