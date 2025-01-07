package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/cloudposse/atmos/pkg/version"
)

var checkFlag bool

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Display the version of Atmos you are running and check for updates",
	Long:    `This command prints the CLI version`,
	Example: "atmos version",
	Run: func(cmd *cobra.Command, args []string) {
		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err := tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

		u.PrintMessage(fmt.Sprintf("\U0001F47D Atmos %s on %s/%s", version.Version, runtime.GOOS, runtime.GOARCH))
		fmt.Println()

		if checkFlag {
			// Check for the latest Atmos release on GitHub
			latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
			if err == nil && latestReleaseTag != "" {
				if err != nil {
					u.LogWarning(schema.AtmosConfiguration{}, fmt.Sprintf("Failed to check for updates: %v", err))
					return
				}
				if latestReleaseTag == "" {
					u.LogWarning(schema.AtmosConfiguration{}, "No release information available")
					return
				}
				latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
				currentRelease := strings.TrimPrefix(version.Version, "v")
				if latestRelease != currentRelease {
					u.PrintMessageToUpgradeToAtmosLatestRelease(latestRelease)
				}
			}
			return
		}

		// Check for the cache and print update message
		CheckForAtmosUpdateAndPrintMessage(atmosConfig)
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks after displaying version info")
	RootCmd.AddCommand(versionCmd)
}
