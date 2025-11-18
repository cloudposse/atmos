package exec

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"

	errUtils "github.com/cloudposse/atmos/errors"
	tuiUtils "github.com/cloudposse/atmos/internal/tui/utils"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"

	"github.com/cloudposse/atmos/pkg/version"
)

//go:embed examples/version_format.md
var versionFormatExample string

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
	defer perf.Track(atmosConfig, "exec.NewVersionExec")()

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
	defer perf.Track(nil, "exec.Execute")()

	if format != "" {
		return v.displayVersionInFormat(checkFlag, format)
	}
	// Print a styled Atmos logo to the terminal.
	v.printMessage("")
	err := v.printStyledText("ATMOS")
	if err != nil {
		return errUtils.Build(errUtils.ErrVersionDisplayFailed).
			WithHint("Try using --format json for machine-readable output").
			WithContext("error", err.Error()).
			Err()
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
		// If force checking is enabled, always return true.
		return true
	}

	// If version checking is disabled in the configuration, do nothing.
	if !v.atmosConfig.Version.Check.Enabled {
		return false
	}

	// Load the cache.
	cacheCfg, err := v.loadCacheConfig()
	if err != nil {
		enrichedErr := errUtils.Build(errUtils.ErrVersionCacheLoadFailed).
			WithHint("Cache file may be corrupted or inaccessible").
			WithContext("operation", "load_cache").
			Err()
		log.Warn("Could not load cache", "error", enrichedErr)
		return false
	}

	// Determine if it's time to check for updates based on frequency and last_checked.
	if !v.shouldCheckForUpdates(cacheCfg.LastChecked, v.atmosConfig.Version.Check.Frequency) {
		// Not due for another check yet, so return without printing anything.
		return false
	}

	return true
}

func (v versionExec) GetLatestVersion(forceCheck bool) (string, bool) {
	defer perf.Track(nil, "exec.GetLatestVersion")()

	if !v.isCheckVersionEnabled(forceCheck) {
		return "", false
	}
	// Get the latest Atmos release from GitHub.
	latestReleaseTag, err := v.getLatestGitHubRepoRelease()
	if err != nil {
		enrichedErr := errUtils.Build(errUtils.ErrVersionGitHubAPIFailed).
			WithHint("Check your internet connection").
			WithHint("Verify GitHub is accessible from your network").
			WithHint("GitHub API may be rate-limited (60 requests/hour without auth)").
			WithContext("operation", "fetch_latest_release").
			Err()
		log.Warn("Failed to retrieve latest Atmos release info", "error", enrichedErr)
		return "", false
	}

	if latestReleaseTag == "" {
		enrichedErr := errUtils.Build(errUtils.ErrVersionCheckFailed).
			WithHint("GitHub API returned empty release information").
			WithContext("operation", "parse_release_tag").
			Err()
		log.Warn("No release information available", "error", enrichedErr)
		return "", false
	}

	// Trim "v" prefix to compare versions.
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
		if err := tuiUtils.WriteJSON(version); err != nil {
			return errUtils.Build(errUtils.ErrVersionDisplayFailed).
				WithHint("Check if stdout is writable").
				WithContext("format", "json").
				Err()
		}
		return nil
	case "yaml":
		if err := tuiUtils.WriteYAML(version); err != nil {
			return errUtils.Build(errUtils.ErrVersionDisplayFailed).
				WithHint("Check if stdout is writable").
				WithContext("format", "yaml").
				Err()
		}
		return nil
	default:
		return errUtils.Build(errUtils.ErrVersionFormatInvalid).
			WithExplanationf("The format '%s' is not supported for version output", format).
			WithExample(versionFormatExample).
			WithHint("Use --format json for JSON output").
			WithHint("Use --format yaml for YAML output").
			WithContext("format", format).
			WithExitCode(2). // Usage error
			Err()
	}
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
		u.PrintfMessageToTUI("\n%s You are running the latest version of Atmos\n\n", theme.Styles.Checkmark)
		log.Debug("Version check completed", "version", latestRelease)
	} else {
		v.printMessageToUpgradeToAtmosLatestRelease(latestRelease)
	}
}
