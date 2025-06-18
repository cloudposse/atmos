package exec

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	log "github.com/charmbracelet/log"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	u "github.com/cloudposse/atmos/pkg/utils"
	"gopkg.in/yaml.v2"

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

func (v versionExec) Execute(checkFlag bool, format string) {
	if format != "" {
		v.displayVersionInFormat(format)
		return
	}
	// Print a styled Atmos logo to the terminal
	v.printMessage("")
	err := v.printStyledText("ATMOS")
	if err != nil {
		//nolint:revive
		log.Fatal(err)
	}

	atmosIcon := "\U0001F47D"

	v.printMessage(fmt.Sprintf("%s Atmos %s on %s/%s", atmosIcon, version.Version, runtime.GOOS, runtime.GOARCH))
	v.printMessage("")

	if checkFlag {
		v.checkRelease()
	}
}

type Version struct {
	Version string `json:"version" yaml:"version"`
	OS      string `json:"os" yaml:"os"`
	Arch    string `json:"arch" yaml:"arch"`
}

func (v versionExec) displayVersionInFormat(format string) {
	version := Version{
		Version: version.Version,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
	switch format {
	case "json":
		if data, err := json.MarshalIndent(version, " ", " "); err == nil {
			fmt.Println(string(data))
		}
	case "yaml":
		if data, err := yaml.Marshal(version); err == nil {
			fmt.Println(string(data))
		}
	}
	if format != "json" && format != "yaml" {
		log.Error("Invalid format specified. Supported formats are: json, yaml")
		return
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
