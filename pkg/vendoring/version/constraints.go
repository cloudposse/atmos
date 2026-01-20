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

// ResolveVersionConstraints applies version constraints to filter a list of available versions.
// Returns the latest version that satisfies all constraints, or an error if no version matches.
func ResolveVersionConstraints(
	availableVersions []string,
	constraints *schema.VendorConstraints,
) (string, error) {
	defer perf.Track(nil, "version.ResolveVersionConstraints")()

	if constraints == nil {
		// No constraints - return latest version.
		if len(availableVersions) == 0 {
			return "", errUtils.ErrNoVersionsAvailable
		}
		return SelectLatestVersion(availableVersions)
	}

	// Filter through constraint pipeline.
	filtered := availableVersions

	// Step 1: Filter by semver constraint.
	if constraints.Version != "" {
		var err error
		filtered, err = FilterBySemverConstraint(filtered, constraints.Version)
		if err != nil {
			return "", err
		}
	}

	// Step 2: Filter excluded versions.
	if len(constraints.ExcludedVersions) > 0 {
		filtered = FilterExcludedVersions(filtered, constraints.ExcludedVersions)
	}

	// Step 3: Filter pre-releases.
	if constraints.NoPrereleases {
		filtered = FilterPrereleases(filtered)
	}

	// Step 4: Select latest from remaining versions.
	if len(filtered) == 0 {
		return "", errUtils.ErrNoVersionsMatchConstraints
	}

	return SelectLatestVersion(filtered)
}

// FilterBySemverConstraint filters versions by semantic version constraint.
func FilterBySemverConstraint(versions []string, constraint string) ([]string, error) {
	defer perf.Track(nil, "version.FilterBySemverConstraint")()

	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, errors.Join(
			errUtils.ErrInvalidSemverConstraint,
			fmt.Errorf("invalid semver constraint %q: %w", constraint, err),
		)
	}

	var filtered []string
	for _, v := range versions {
		// Try parsing as semver.
		sv, err := semver.NewVersion(v)
		if err != nil {
			// Not a valid semver - skip it.
			continue
		}

		if c.Check(sv) {
			filtered = append(filtered, v)
		}
	}

	return filtered, nil
}

// FilterExcludedVersions filters out excluded versions (supports wildcards).
func FilterExcludedVersions(versions []string, excluded []string) []string {
	defer perf.Track(nil, "version.FilterExcludedVersions")()

	var filtered []string

	for _, v := range versions {
		exclude := false
		for _, pattern := range excluded {
			if MatchesWildcard(v, pattern) {
				exclude = true
				break
			}
		}
		if !exclude {
			filtered = append(filtered, v)
		}
	}

	return filtered
}

// MatchesWildcard checks if a version matches a wildcard pattern.
// Supports patterns like "1.5.*" or exact matches like "1.2.3".
func MatchesWildcard(version, pattern string) bool {
	defer perf.Track(nil, "version.MatchesWildcard")()

	// Exact match.
	if version == pattern {
		return true
	}

	// Wildcard pattern.
	if strings.Contains(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(version, prefix)
	}

	return false
}

// FilterPrereleases filters out pre-release versions.
func FilterPrereleases(versions []string) []string {
	defer perf.Track(nil, "version.FilterPrereleases")()

	var filtered []string

	for _, v := range versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			// Not a valid semver - keep it.
			filtered = append(filtered, v)
			continue
		}

		// Keep if not a pre-release.
		if sv.Prerelease() == "" {
			filtered = append(filtered, v)
		}
	}

	return filtered
}

// SelectLatestVersion selects the latest version from a list using semver comparison.
func SelectLatestVersion(versions []string) (string, error) {
	defer perf.Track(nil, "version.SelectLatestVersion")()

	if len(versions) == 0 {
		return "", errUtils.ErrNoVersionsAvailable
	}

	var latest *semver.Version
	var latestStr string

	for _, v := range versions {
		sv, err := semver.NewVersion(v)
		if err != nil {
			// Not a valid semver - skip it.
			continue
		}

		if latest == nil || sv.GreaterThan(latest) {
			latest = sv
			latestStr = v
		}
	}

	if latest == nil {
		// No valid semver found - return first version as fallback.
		return versions[0], nil
	}

	return latestStr, nil
}
