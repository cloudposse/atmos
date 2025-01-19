package cmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
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
		// Get the log level from command line flag
		logsLevel, err := cmd.Flags().GetString("logs-level")
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

		// Initialize atmosConfig with the log level from flags
		info := schema.ConfigAndStacksInfo{
			LogsLevel: logsLevel,
		}
		atmosConfig, err := cfg.InitCliConfig(info, false)
		if err != nil {
			u.LogErrorAndExit(schema.AtmosConfiguration{}, err)
		}

		// Print a styled Atmos logo to the terminal
		fmt.Println()
		err = tuiUtils.PrintStyledText("ATMOS")
		if err != nil {
			u.LogErrorAndExit(atmosConfig, err)
		}

		atmosIcon := "\U0001F47D"

		u.PrintMessage(fmt.Sprintf("%s Atmos %s on %s/%s", atmosIcon, version.Version, runtime.GOOS, runtime.GOARCH))
		fmt.Println()

		if checkFlag {
			u.LogDebug(atmosConfig, "Checking for latest Atmos release...")
			// Check for the latest Atmos release on GitHub
			latestReleaseTag, err := u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
			if err == nil && latestReleaseTag != "" {
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

				if latestRelease == currentRelease {
					u.LogDebug(atmosConfig, fmt.Sprintf("Current version %s is the latest version", currentRelease))
					u.PrintMessage(fmt.Sprintf("You are running the latest version of Atmos (%s)", latestRelease))
				} else {
					u.LogDebug(atmosConfig, fmt.Sprintf("New version %s is available (current version: %s)", latestRelease, currentRelease))
					u.PrintMessageToUpgradeToAtmosLatestRelease(latestRelease)
				}
			}
			return
		}

		// Check for the cache and print update message
		u.LogDebug(atmosConfig, "Checking for updates from cache...")
		CheckForAtmosUpdateAndPrintMessage(atmosConfig)
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks after displaying version info")
	RootCmd.AddCommand(versionCmd)
}
