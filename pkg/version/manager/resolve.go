package manager

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// ResolveTarget returns the desired concrete version for an entry.
func ResolveTarget(atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry) (string, error) {
	defer perf.Track(atmosConfig, "manager.ResolveTarget")()

	if entry.Desired == "" {
		return "", fmt.Errorf("%w: %s", ErrDesiredVersionRequired, entry.Name)
	}
	if entry.Datasource == "toolchain" || entry.Ecosystem == "toolchain" {
		return resolveToolchain(atmosConfig, entry)
	}
	if !looksLikeConstraint(entry.Desired) {
		return entry.Desired, nil
	}
	return "", fmt.Errorf("%w: datasource %q does not resolve constraints yet; use a concrete desired version", ErrResolverUnsupported, entry.Datasource)
}

// resolveToolchain resolves a toolchain package version via the Aqua registry.
func resolveToolchain(atmosConfig *schema.AtmosConfiguration, entry *EffectiveEntry) (string, error) {
	installer := toolchain.NewInstaller(toolchain.WithAtmosConfig(atmosConfig))
	owner, repo, err := installer.ParseToolSpec(entry.Package)
	if err != nil {
		return "", err
	}
	reg := toolchain.NewAquaRegistry()
	if entry.Desired == "latest" {
		return reg.GetLatestVersion(owner, repo)
	}
	if !looksLikeConstraint(entry.Desired) {
		return entry.Desired, nil
	}
	lister, ok := reg.(interface {
		GetAvailableVersions(owner, repo string) ([]string, error)
	})
	if !ok {
		return "", fmt.Errorf("%w: toolchain registry cannot list versions", ErrResolverUnsupported)
	}
	versions, err := lister.GetAvailableVersions(owner, repo)
	if err != nil {
		return "", err
	}
	constraint, err := semver.NewConstraint(entry.Desired)
	if err != nil {
		return "", err
	}
	return highestMatch(versions, constraint)
}

// highestMatch returns the highest candidate version satisfying the constraint.
func highestMatch(candidates []string, constraint *semver.Constraints) (string, error) {
	var matched []*semver.Version
	for _, candidate := range candidates {
		version, err := semver.NewVersion(strings.TrimPrefix(candidate, "v"))
		if err != nil {
			continue
		}
		if constraint.Check(version) {
			matched = append(matched, version)
		}
	}
	if len(matched) == 0 {
		return "", ErrNoVersionMatch
	}
	sort.Sort(sort.Reverse(semver.Collection(matched)))
	return matched[0].Original(), nil
}

// looksLikeConstraint reports whether a desired version uses constraint syntax
// rather than naming a concrete version.
func looksLikeConstraint(version string) bool {
	if version == "" || version == "latest" {
		return false
	}
	if strings.ContainsAny(version, "^~<>= ") || strings.Contains(version, "||") {
		return true
	}
	_, err := semver.NewVersion(strings.TrimPrefix(version, "v"))
	return err != nil && strings.Contains(version, ",")
}
