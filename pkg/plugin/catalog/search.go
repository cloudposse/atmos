package catalog

import (
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Filter constrains catalog discovery without loading executable payloads.
type Filter struct{ Query, Capability, Runtime string }

// Search returns matching static entries. An empty catalog produces an empty list.
func Search(file *File, filter Filter) []Entry {
	defer perf.Track(nil, "catalog.Search")()

	result := []Entry{}
	for itemIndex := range file.Spec.Entries {
		entry := file.Spec.Entries[itemIndex]
		if filter.Capability != "" && !slices.Contains(entry.Capabilities, filter.Capability) {
			continue
		}
		if filter.Runtime != "" && entry.Runtime != filter.Runtime {
			continue
		}
		if filter.Query != "" && !strings.Contains(strings.ToLower(entry.Name+" "+entry.Description), strings.ToLower(filter.Query)) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

// Select chooses the newest non-yanked version for the requested platform.
//
//nolint:lintroller // Trivial accessor, pure lookup, factory option, or delegation; exempt under repository tracking policy.
func Select(entry *Entry, constraint, platform string) (*Version, error) {
	requested, err := semver.NewConstraint(constraint)
	if err != nil {
		return nil, err
	}
	var result *Version
	var selected *semver.Version
	for _, version := range entry.Versions {
		candidate, err := semver.StrictNewVersion(version.Version)
		if err != nil {
			return nil, err
		}
		if version.Yanked != nil || !requested.Check(candidate) || !slices.Contains(version.Platforms, platform) {
			continue
		}
		if selected == nil || candidate.GreaterThan(selected) {
			selected, result = candidate, &version
		}
	}
	if result == nil {
		return nil, errUtils.Build(errUtils.ErrPluginVersionNotFound).
			WithExplanationf("%s has no non-yanked release for %s that satisfies `%s`.", entry.Name, platform, constraint).
			WithHint("Loosen the version constraint, or ask the publisher for a release that supports this platform.").Err()
	}
	return result, nil
}
