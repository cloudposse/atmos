package version

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/cloudposse/atmos/pkg/perf"
)

// commitSHAPattern matches full and short Git commit SHAs.
var commitSHAPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// ParseSemVer parses a version string as a semantic version, tolerating a
// leading `v`/`V` prefix.
func ParseSemVer(version string) (*semver.Version, error) {
	defer perf.Track(nil, "version.ParseSemVer")()

	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return semver.NewVersion(version)
}

// FindLatestSemVerTag returns the highest semantic version among tags and the
// original tag string for it. Non-semver tags are ignored.
func FindLatestSemVerTag(tags []string) (*semver.Version, string) {
	defer perf.Track(nil, "version.FindLatestSemVerTag")()

	var latestVer *semver.Version
	var latestTag string

	for _, tag := range tags {
		ver, err := ParseSemVer(tag)
		if err != nil {
			continue
		}
		if latestVer == nil || ver.GreaterThan(latestVer) {
			latestVer = ver
			latestTag = tag
		}
	}
	return latestVer, latestTag
}

// IsValidCommitSHA reports whether ref looks like a Git commit SHA (7-40 hex chars).
func IsValidCommitSHA(ref string) bool {
	defer perf.Track(nil, "version.IsValidCommitSHA")()

	return commitSHAPattern.MatchString(ref)
}

// IsTemplatedVersion reports whether a version string contains Go template
// syntax (e.g. `{{.Version}}`) and therefore must be skipped by update.
func IsTemplatedVersion(version string) bool {
	defer perf.Track(nil, "version.IsTemplatedVersion")()

	return strings.Contains(version, "{{")
}
