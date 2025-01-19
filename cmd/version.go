package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

var checkFlag bool

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Display the version of Atmos you are running and check for updates",
	Long:    `This command shows the version of the Atmos CLI you are currently running and checks if a newer version is available. Use this command to verify your installation and ensure you are up to date.`,
	Example: "atmos version",
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(atmosConfig, err)
		}

		atmosIcon := "\U0001F47D"

		u.PrintMessage(fmt.Sprintf("%s Atmos %s on %s/%s", atmosIcon, version.Version, runtime.GOOS, runtime.GOARCH))
		fmt.Println()

		if checkFlag {
			u.LogDebug(atmosConfig, "Checking for latest Atmos release on GitHub")
			// Check for the latest Atmos release on GitHub
			latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos", atmosConfig)
			if err != nil {
				u.LogWarning(atmosConfig, fmt.Sprintf("Failed to check for updates: %v", err))
				return
			}
			if latestReleaseTag == "" {
				u.LogWarning(atmosConfig, "No release information available")
				return
			}
			latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
			currentRelease := strings.TrimPrefix(version.Version, "v")
			u.LogDebug(atmosConfig, fmt.Sprintf("Current version: %s, Latest version: %s", currentRelease, latestRelease))
			if latestRelease != currentRelease {
				u.PrintMessageToUpgradeToAtmosLatestRelease(latestRelease)
			} else {
				u.LogDebug(atmosConfig, "Atmos is up to date")
			}
			return
		}

		// Check for the cache and print update message
		u.LogDebug(atmosConfig, "Checking for updates using cache")
		CheckForAtmosUpdateAndPrintMessage(atmosConfig)
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks after displaying version info")
	RootCmd.AddCommand(versionCmd)
}
