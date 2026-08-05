package resolver

import (
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/sirupsen/logrus"
	updatecliversion "github.com/updatecli/updatecli/pkg/plugins/utils/version"

	"github.com/cloudposse/atmos/pkg/perf"
)

func init() {
	// The updatecli version filter logs informational messages through the
	// global logrus logger on every search. Nothing else in Atmos uses logrus,
	// so silence it; errors surface through return values.
	logrus.SetOutput(io.Discard)
}

// Select returns the best candidate for the desired version expression after
// applying include, exclude, and prerelease rules. Desired takes one of three
// forms: "latest", a SemVer constraint (e.g. "~1.10", ">= 1.2, < 2"), or a
// concrete version (returned when present in the candidate list). Prerelease
// candidates are excluded unless prerelease is true. Include and exclude
// entries are glob or substring patterns matched against candidate versions.
func Select(candidates []Candidate, desired string, include, exclude []string, prerelease bool) (Candidate, error) {
	defer perf.Track(nil, "resolver.Select")()

	filtered := applyRules(candidates, include, exclude, prerelease)
	if len(filtered) == 0 {
		return Candidate{}, fmt.Errorf("%w: no candidates remain after include/exclude/prerelease rules", ErrNoVersionMatch)
	}

	if desired != "" && desired != "latest" && !LooksLikeConstraint(desired) {
		for i := range filtered {
			if filtered[i].Version == desired {
				return filtered[i], nil
			}
		}
		return Candidate{}, fmt.Errorf("%w: %s", ErrNoVersionMatch, desired)
	}

	if desired == "" || desired == "latest" {
		return selectLatest(filtered), nil
	}
	return selectConstraint(filtered, desired)
}

// AllowsVersion reports whether one concrete version passes the include,
// exclude, and prerelease rules.
func AllowsVersion(version string, include, exclude []string, prerelease bool) bool {
	defer perf.Track(nil, "resolver.AllowsVersion")()

	return allowsCandidate(&Candidate{Version: version}, include, exclude, prerelease)
}

func allowsCandidate(candidate *Candidate, include, exclude []string, prerelease bool) bool {
	if !prerelease && isPrerelease(candidate) {
		return false
	}
	if len(include) > 0 && !matchesVersionPattern(include, candidate.Version) {
		return false
	}
	return !matchesVersionPattern(exclude, candidate.Version)
}

// selectLatest returns the highest SemVer candidate. Prerelease filtering has
// already happened via the policy rules, so prereleases that survive count.
// When no candidate parses as SemVer, it falls back to the datasource's own
// ordering (newest first).
func selectLatest(candidates []Candidate) Candidate {
	best := -1
	var bestVersion *semver.Version
	for i := range candidates {
		parsed, err := semver.NewVersion(strings.TrimPrefix(candidates[i].Version, "v"))
		if err != nil {
			continue
		}
		if bestVersion == nil || parsed.GreaterThan(bestVersion) {
			best = i
			bestVersion = parsed
		}
	}
	if best < 0 {
		return candidates[0]
	}
	return candidates[best]
}

// selectConstraint returns the highest candidate satisfying a SemVer
// constraint, using the updatecli version filter engine. Per SemVer semantics
// (Masterminds), constraints match prerelease versions only when the
// constraint itself carries a prerelease bound (e.g. "~1.10-0").
func selectConstraint(candidates []Candidate, desired string) (Candidate, error) {
	filter, err := updatecliversion.Filter{
		Kind:    updatecliversion.SEMVERVERSIONKIND,
		Pattern: desired,
	}.Init()
	if err != nil {
		return Candidate{}, err
	}

	versions := make([]string, len(candidates))
	byVersion := make(map[string]Candidate, len(candidates))
	for i := range candidates {
		versions[i] = candidates[i].Version
		byVersion[candidates[i].Version] = candidates[i]
	}
	found, err := filter.Search(versions)
	if err != nil {
		return Candidate{}, fmt.Errorf("%w: %s: %w", ErrNoVersionMatch, desired, err)
	}
	selected, ok := byVersion[found.OriginalVersion]
	if !ok {
		return Candidate{}, fmt.Errorf("%w: %s", ErrNoVersionMatch, desired)
	}
	return selected, nil
}

// applyRules drops candidates rejected by the include, exclude, and prerelease
// rules.
func applyRules(candidates []Candidate, include, exclude []string, prerelease bool) []Candidate {
	var result []Candidate
	for i := range candidates {
		if !allowsCandidate(&candidates[i], include, exclude, prerelease) {
			continue
		}
		result = append(result, candidates[i])
	}
	return result
}

// isPrerelease reports whether a candidate is a prerelease, either by the
// datasource's flag or by a SemVer prerelease suffix (e.g. 1.2.3-rc.1).
func isPrerelease(candidate *Candidate) bool {
	if candidate.Prerelease {
		return true
	}
	parsed, err := semver.NewVersion(strings.TrimPrefix(candidate.Version, "v"))
	return err == nil && parsed.Prerelease() != ""
}

// matchesVersionPattern reports whether a version matches any pattern, using
// glob patterns with a substring fallback.
func matchesVersionPattern(patterns []string, version string) bool {
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, version); ok {
			return true
		}
		if strings.Contains(version, pattern) {
			return true
		}
	}
	return false
}

// LooksLikeConstraint reports whether a desired version uses constraint syntax
// rather than naming a concrete version.
func LooksLikeConstraint(version string) bool {
	defer perf.Track(nil, "resolver.LooksLikeConstraint")()

	if version == "" || version == "latest" {
		return false
	}
	if strings.ContainsAny(version, "^~<>= ") || strings.Contains(version, "||") {
		return true
	}
	_, err := semver.NewVersion(strings.TrimPrefix(version, "v"))
	return err != nil && strings.Contains(version, ",")
}
