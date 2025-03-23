package exec

import (
	"fmt"
	"runtime"
	"strings"

	log "github.com/charmbracelet/log"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/utils"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/cloudposse/atmos/pkg/version"
)

type versionExec struct {
	printStyledText                           func(string) error
	getLatestGitHubRepoRelease                func(string, string) (string, error)
	printMessage                              func(string)
	printMessageToUpgradeToAtmosLatestRelease func(string)
}

func NewVersionExec() *versionExec {
	return &versionExec{
		printStyledText:                           tuiUtils.PrintStyledText,
		getLatestGitHubRepoRelease:                u.GetLatestGitHubRepoRelease,
		printMessage:                              u.PrintMessage,
		printMessageToUpgradeToAtmosLatestRelease: u.PrintMessageToUpgradeToAtmosLatestRelease,
	}
}

func (v versionExec) Execute(checkFlag bool) {
	// Print a styled Atmos logo to the terminal
	v.printMessage("")
	err := v.printStyledText("ATMOS")
	if err != nil {
		//nolint:revive
		log.Error(err)
		utils.OsExit(1)
	}

	atmosIcon := "\U0001F47D"

	v.printMessage(fmt.Sprintf("%s Atmos %s on %s/%s", atmosIcon, version.Version, runtime.GOOS, runtime.GOARCH))
	v.printMessage("")

	if checkFlag {
		v.checkRelease()
	}
}

func (v versionExec) checkRelease() {
	// Check for the latest Atmos release on GitHub
	latestReleaseTag, err := v.getLatestGitHubRepoRelease("cloudposse", "atmos")
	if err != nil || latestReleaseTag == "" {
		log.Debug("Did not get release tag", "err", err, "latestReleaseTag", latestReleaseTag)
		return
	}
	latestRelease := strings.TrimPrefix(latestReleaseTag, "v")
	currentRelease := strings.TrimPrefix(version.Version, "v")

	if latestRelease == currentRelease {
		log.Info("You are running the latest version of Atmos", "version", latestRelease)
	} else {
		v.printMessageToUpgradeToAtmosLatestRelease(latestRelease)
	}
}
