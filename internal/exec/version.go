package exec

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/log"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/cloudposse/atmos/pkg/version"
)

type versionExec struct {
	printStyledText            func(string) error
	getLatestGitHubRepoRelease func(string, string) (string, error)
}

func NewVersionExec() *versionExec {
	return &versionExec{
		printStyledText:            tuiUtils.PrintStyledText,
		getLatestGitHubRepoRelease: u.GetLatestGitHubRepoRelease,
	}
}

func (v versionExec) Execute(checkFlag bool) {
	// Print a styled Atmos logo to the terminal
	fmt.Println()
	err := v.printStyledText("ATMOS")
	if err != nil {
		log.Fatal(err)
	}

	atmosIcon := "\U0001F47D"

	u.PrintMessage(fmt.Sprintf("%s Atmos %s on %s/%s", atmosIcon, version.Version, runtime.GOOS, runtime.GOARCH))
	fmt.Println()

	if checkFlag {
		// Check for the latest Atmos release on GitHub
		latestReleaseTag, err := v.getLatestGitHubRepoRelease("cloudposse", "atmos")
		if err == nil && latestReleaseTag != "" {
			if err != nil {
				log.Warn("Failed to check for updates", "err", err)
				return
			}
			if latestReleaseTag == "" {
				log.Warn("No release information available")
				return
			}
			latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
			currentRelease := strings.TrimPrefix(version.Version, "v")

			if latestRelease == currentRelease {
				log.Warn("You are running the latest version of Atmos", "version", latestRelease)
			} else {
				u.PrintMessageToUpgradeToAtmosLatestRelease(latestRelease)
			}
		} else {
			log.Debug("Did not get release tag", "err", err, "latestReleaseTag", latestReleaseTag)
		}
		return
	}
}
