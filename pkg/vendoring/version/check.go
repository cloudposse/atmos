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

// IsSemverConstraint reports whether v is a semver *range* expression (e.g. "^1.0.0", "~1.2.3",
// ">=1.0.0 <2.0.0", "*") as opposed to an exact version. An exact version -- even one that would
// also technically parse as a (degenerate) constraint, such as a bare "1.2.3" -- always returns
// false: exact pins take priority, so every existing manifest's "version: is templated verbatim"
// behavior stays byte-for-byte unchanged. An opaque literal that is neither a valid exact semver
// nor a valid constraint (a commit SHA, a branch name, "") also returns false, falling through to
// today's literal-templating behavior rather than erroring.
func IsSemverConstraint(v string) bool {
	defer perf.Track(nil, "version.IsSemverConstraint")()

	if v == "" {
		return false
	}
	if _, err := semver.NewVersion(v); err == nil {
		return false
	}
	_, err := semver.NewConstraint(v)
	return err == nil
}
