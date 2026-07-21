package version

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ResolveVersionConstraints applies the configured constraints to a list of
// available versions and returns the latest version that satisfies them all.
func ResolveVersionConstraints(availableVersions []string, constraints *schema.VendorConstraints) (string, error) {
	defer perf.Track(nil, "version.ResolveVersionConstraints")()

	if constraints == nil {
		if len(availableVersions) == 0 {
			return "", errUtils.ErrNoVersionsAvailable
		}
		return SelectLatestVersion(availableVersions)
	}

	filtered := availableVersions

	if constraints.Version != "" {
		var err error
		filtered, err = FilterBySemverConstraint(filtered, constraints.Version)
		if err != nil {
			return "", err
		}
	}

	if len(constraints.ExcludedVersions) > 0 {
		filtered = FilterExcludedVersions(filtered, constraints.ExcludedVersions)
	}

	if constraints.NoPrereleases {
		filtered = FilterPrereleases(filtered)
	}

	if len(filtered) == 0 {
		return "", errUtils.ErrNoVersionsMatchConstraints
	}
	return SelectLatestVersion(filtered)
}

// FilterBySemverConstraint keeps only versions matching the semver constraint.
func FilterBySemverConstraint(versions []string, constraint string) ([]string, error) {
	defer perf.Track(nil, "version.FilterBySemverConstraint")()

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, errors.Join(errUtils.ErrInvalidSemverConstraint,
			fmt.Errorf("invalid semver constraint %q: %w", constraint, err))
	}

	var filtered []string
	for _, v := range versions {
		sv, parseErr := ParseSemVer(v)
		if parseErr != nil {
			continue
		}
		if c.Check(sv) {
			filtered = append(filtered, v)
		}
	}
	return filtered, nil
}

// FilterExcludedVersions removes versions matching any exclusion pattern.
func FilterExcludedVersions(versions, excluded []string) []string {
	defer perf.Track(nil, "version.FilterExcludedVersions")()

	var filtered []string
	for _, v := range versions {
		if !isExcluded(v, excluded) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// isExcluded reports whether v matches any exclusion pattern.
func isExcluded(v string, excluded []string) bool {
	for _, pattern := range excluded {
		if MatchesWildcard(v, pattern) {
			return true
		}
	}
	return false
}

// MatchesWildcard reports whether version matches pattern, which may be an exact
// value or a trailing-wildcard prefix such as "1.5.*".
func MatchesWildcard(version, pattern string) bool {
	defer perf.Track(nil, "version.MatchesWildcard")()

	if version == pattern {
		return true
	}
	if strings.Contains(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(version, prefix)
	}
	return false
}

// FilterPrereleases removes pre-release versions; non-semver values are kept.
func FilterPrereleases(versions []string) []string {
	defer perf.Track(nil, "version.FilterPrereleases")()

	var filtered []string
	for _, v := range versions {
		sv, err := ParseSemVer(v)
		if err != nil {
			filtered = append(filtered, v)
			continue
		}
		if sv.Prerelease() == "" {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// SelectLatestVersion returns the highest semver value from versions, falling
// back to the first entry when none parse as semver.
func SelectLatestVersion(versions []string) (string, error) {
	defer perf.Track(nil, "version.SelectLatestVersion")()

	if len(versions) == 0 {
		return "", errUtils.ErrNoVersionsAvailable
	}

	var latest *semver.Version
	var latestStr string
	for _, v := range versions {
		sv, err := ParseSemVer(v)
		if err != nil {
			continue
		}
		switch {
		case latest == nil || sv.GreaterThan(latest):
			latest, latestStr = sv, v
		case sv.Equal(latest) && len(v) > len(latestStr):
			// Deterministic tie-break: prefer the more specific tag (e.g. "v3.0.0"
			// over a moving "v3") so repeated runs select the same version.
			latestStr = v
		}
	}

	if latest == nil {
		return versions[0], nil
	}
	return latestStr, nil
}
