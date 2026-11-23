package upgrade

import (
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CompareVersions orders current against latest by semantic version, accepting a leading "v".
// It returns -1, 0, or 1; ok is false when either version is unparsable (for example, the
// development version "test"). A prerelease sorts before its release.
func CompareVersions(current, latest string) (order int, ok bool) {
	defer perf.Track(nil, "upgrade.CompareVersions")()

	currentVersion, err := semver.NewVersion(current)
	if err != nil {
		return 0, false
	}
	latestVersion, err := semver.NewVersion(latest)
	if err != nil {
		return 0, false
	}
	return currentVersion.Compare(latestVersion), true
}

// NewerRelease returns latest without a leading "v" only when it is strictly newer than current.
// Equal, older, and unparsable versions never produce an update suggestion, so a newer build or
// prerelease is never told to "update" to an older release.
func NewerRelease(current, latest string) (string, bool) {
	defer perf.Track(nil, "upgrade.NewerRelease")()

	if order, ok := CompareVersions(current, latest); !ok || order >= 0 {
		return "", false
	}
	return strings.TrimPrefix(latest, "v"), true
}
