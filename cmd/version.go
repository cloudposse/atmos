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
			u.LogErrorAndExit(err)
		}

		atmosIcon := "\U0001F47D"

		u.PrintMessage(fmt.Sprintf("%s Atmos %s on %s/%s", atmosIcon, version.Version, runtime.GOOS, runtime.GOARCH))
		fmt.Println()

		// Only check for updates when explicitly requested
		if checkFlag {
			u.LogDebug(atmosConfig, "Checking for latest Atmos release on Github")
			// Check for the latest Atmos release on GitHub
			latestReleaseTag, err := u.GetLatestGitHubRepoRelease(atmosConfig, "cloudposse", "atmos")
			if err == nil && latestReleaseTag != "" {
				if err != nil {
					u.LogWarning(fmt.Sprintf("Failed to check for updates: %v", err))
					return
				}
				if latestReleaseTag == "" {
					u.LogWarning("No release information available")
					return
				}
				latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
				currentRelease := strings.TrimPrefix(version.Version, "v")

				u.LogDebug(atmosConfig, fmt.Sprintf("Latest release tag: v%s", latestRelease))
				u.LogDebug(atmosConfig, fmt.Sprintf("Current version: %s, Latest version: %s", currentRelease, latestRelease))

				if latestRelease == currentRelease {
					u.PrintMessage(fmt.Sprintf("You are running the latest version of Atmos (%s)", latestRelease))
				} else {
					u.PrintMessageToUpgradeToAtmosLatestRelease(latestRelease)
				}
			}

			// Check for the cache and print update message
			u.LogDebug(atmosConfig, "Checking for updates from cache...")
			CheckForAtmosUpdateAndPrintMessage(atmosConfig)
		}
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks after displaying version info")
	RootCmd.AddCommand(versionCmd)
}
