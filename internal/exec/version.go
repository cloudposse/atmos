package exec

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	log "github.com/charmbracelet/log"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"gopkg.in/yaml.v2"

	"github.com/cloudposse/atmos/pkg/version"
)

type versionExec struct {
	atmosConfig                               *schema.AtmosConfiguration
	printStyledText                           func(string) error
	getLatestGitHubRepoRelease                func() (string, error)
	printMessage                              func(string)
	printMessageToUpgradeToAtmosLatestRelease func(string)
	loadCacheConfig                           func() (cfg.CacheConfig, error)
	shouldCheckForUpdates                     func(lastChecked int64, frequency string) bool
}

func NewVersionExec(atmosConfig *schema.AtmosConfiguration) *versionExec {
	return &versionExec{
		atmosConfig:     atmosConfig,
		printStyledText: tuiUtils.PrintStyledText,
		getLatestGitHubRepoRelease: func() (string, error) {
			return u.GetLatestGitHubRepoRelease("cloudposse", "atmos")
		},
		printMessage: u.PrintMessage,
		printMessageToUpgradeToAtmosLatestRelease: u.PrintMessageToUpgradeToAtmosLatestRelease,
		loadCacheConfig:       cfg.LoadCache,
		shouldCheckForUpdates: cfg.ShouldCheckForUpdates,
	}
}

func (v versionExec) Execute(checkFlag bool, format string) error {
	if format != "" {
		return v.displayVersionInFormat(checkFlag, format)
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
		return nil
	}

	if updatedVersion, ok := v.GetLatestVersion(false); ok {
		u.PrintMessageToUpgradeToAtmosLatestRelease(updatedVersion)
	}
	return nil
}

type Version struct {
	Version       string `json:"version" yaml:"version"`
	OS            string `json:"os" yaml:"os"`
	Arch          string `json:"arch" yaml:"arch"`
	UpdateVersion string `json:"update_version,omitempty" yaml:"update_version,omitempty"`
}

func (v versionExec) isCheckVersionEnabled(forceCheck bool) bool {
	if forceCheck {
		// If force checking is enabled, always return true
		return true
	}

	// If version checking is disabled in the configuration, do nothing
	if !v.atmosConfig.Version.Check.Enabled {
		return false
	}

	// Load the cache
	cacheCfg, err := v.loadCacheConfig()
	if err != nil {
		log.Warn("Could not load cache", err)
		return false
	}

	// Determine if it's time to check for updates based on frequency and last_checked
	if !v.shouldCheckForUpdates(cacheCfg.LastChecked, v.atmosConfig.Version.Check.Frequency) {
		// Not due for another check yet, so return without printing anything
		return false
	}

	return true
}

func (v versionExec) GetLatestVersion(forceCheck bool) (string, bool) {
	if !v.isCheckVersionEnabled(forceCheck) {
		return "", false
	}
	// Get the latest Atmos release from GitHub
	latestReleaseTag, err := v.getLatestGitHubRepoRelease()
	if err != nil {
		log.Warn("Failed to retrieve latest Atmos release info", err)
		return "", false
	}

	if latestReleaseTag == "" {
		log.Warn("No release information available")
		return "", false
	}

	// Trim "v" prefix to compare versions
	latestVersion := strings.TrimPrefix(latestReleaseTag, "v")
	currentVersion := strings.TrimPrefix(version.Version, "v")

	if latestVersion != currentVersion {
		return latestVersion, true
	}
	return "", false
}

func (v versionExec) displayVersionInFormat(forceCheck bool, format string) error {
	version := Version{
		Version: version.Version,
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
	if v, ok := v.GetLatestVersion(forceCheck); ok {
		version.UpdateVersion = strings.TrimPrefix(v, "v")
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
	default:
		return ErrInvalidFormat
	}
	return nil
}

func (v versionExec) checkRelease() {
	// Check for the latest Atmos release on GitHub
	latestReleaseTag, err := v.getLatestGitHubRepoRelease()
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
