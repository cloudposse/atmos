// Package manager implements the Atmos Version Tracker: externally managed
// versions declared in atmos.yaml, resolved into a lock file, and consumed at
// runtime via the !version YAML function and the .version template context.
package manager

import (
	"fmt"
	"path"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultTrack is the implicit track name used when none is configured.
	DefaultTrack = "default"
	// DefaultLockFile is the default managed versions lock file name.
	DefaultLockFile = "versions.lock.yaml"
	lockVersion     = 1
)

var (
	ErrTrackNotFound          = errUtils.ErrVersionTrackNotFound
	ErrVersionNotFound        = errUtils.ErrVersionNotFound
	ErrVersionNotLocked       = errUtils.ErrVersionNotLocked
	ErrTrackNotVerified       = errUtils.ErrVersionTrackNotVerified
	ErrDesiredVersionRequired = errUtils.ErrDesiredVersionRequired
)

// EffectiveEntry is a version entry after defaults and groups are applied.
type EffectiveEntry struct {
	Name       string                     `yaml:"name" json:"name"`
	Ecosystem  string                     `yaml:"ecosystem,omitempty" json:"ecosystem,omitempty"`
	Datasource string                     `yaml:"datasource,omitempty" json:"datasource,omitempty"`
	Provider   string                     `yaml:"provider,omitempty" json:"provider,omitempty"`
	Package    string                     `yaml:"package,omitempty" json:"package,omitempty"`
	Desired    string                     `yaml:"desired,omitempty" json:"desired,omitempty"`
	Group      string                     `yaml:"group,omitempty" json:"group,omitempty"`
	Update     schema.VersionUpdatePolicy `yaml:"update,omitempty" json:"update,omitempty"`
	Include    []string                   `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude    []string                   `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Prerelease bool                       `yaml:"prerelease,omitempty" json:"prerelease,omitempty"`
	Labels     []string                   `yaml:"labels,omitempty" json:"labels,omitempty"`
	Locked     string                     `yaml:"locked,omitempty" json:"locked,omitempty"`
}

// EffectiveTrack returns the requested track, the config default, or "default".
func EffectiveTrack(atmosConfig *schema.AtmosConfiguration, requested string) string {
	defer perf.Track(atmosConfig, "manager.EffectiveTrack")()

	if requested != "" {
		return requested
	}
	if atmosConfig != nil && atmosConfig.Version.Track != "" {
		return atmosConfig.Version.Track
	}
	return DefaultTrack
}

// EffectiveTrackFromStack returns the stack-asserted track when present.
func EffectiveTrackFromStack(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) string {
	defer perf.Track(atmosConfig, "manager.EffectiveTrackFromStack")()

	if stackInfo != nil {
		if versionSection, ok := stackInfo.StackSection["version"].(map[string]any); ok {
			if track, ok := versionSection["track"].(string); ok && track != "" {
				return track
			}
		}
	}
	return EffectiveTrack(atmosConfig, "")
}

// TrackNames returns sorted configured track names. It also includes the
// implicit default track (the one EffectiveTrack resolves to) when it has no
// entry in version.tracks but is usable via the base dependency catalog, so
// callers see the same set of usable tracks that EffectiveEntries does.
func TrackNames(atmosConfig *schema.AtmosConfiguration) []string {
	defer perf.Track(atmosConfig, "manager.TrackNames")()

	names := make([]string, 0, len(atmosConfig.Version.Tracks)+1)
	for name := range atmosConfig.Version.Tracks {
		names = append(names, name)
	}
	implicit := EffectiveTrack(atmosConfig, "")
	if _, ok := atmosConfig.Version.Tracks[implicit]; !ok && canUseBaseCatalogTrack(atmosConfig, implicit) {
		names = append(names, implicit)
	}
	sort.Strings(names)
	return names
}

// EffectiveEntries returns all versions for a track with defaults and groups applied.
func EffectiveEntries(atmosConfig *schema.AtmosConfiguration, track string) (map[string]EffectiveEntry, error) {
	defer perf.Track(atmosConfig, "manager.EffectiveEntries")()

	if atmosConfig == nil {
		return nil, errUtils.ErrAtmosConfigIsNil
	}
	track = EffectiveTrack(atmosConfig, track)
	versionTrack, ok := atmosConfig.Version.Tracks[track]
	if !ok {
		if !canUseBaseCatalogTrack(atmosConfig, track) {
			return nil, fmt.Errorf("%w: %s", ErrTrackNotFound, track)
		}
		versionTrack = schema.VersionTrack{}
	}

	entries, err := collectTrackEntries(atmosConfig, &versionTrack)
	if err != nil {
		return nil, err
	}

	result := make(map[string]EffectiveEntry, len(entries))
	for name := range entries {
		entry := entries[name]
		result[name] = buildEffectiveEntry(atmosConfig, &versionTrack, name, &entry)
	}
	return result, nil
}

func canUseBaseCatalogTrack(atmosConfig *schema.AtmosConfiguration, track string) bool {
	if len(atmosConfig.Version.Dependencies) == 0 {
		return false
	}
	if atmosConfig.Version.Track != "" {
		return track == atmosConfig.Version.Track
	}
	return track == DefaultTrack
}

// collectTrackEntries merges the base dependency catalog, extended parent
// tracks, and the requested track's dependency overrides.
func collectTrackEntries(atmosConfig *schema.AtmosConfiguration, versionTrack *schema.VersionTrack) (map[string]schema.VersionEntry, error) {
	entries := map[string]schema.VersionEntry{}
	mergeEntrySet(entries, atmosConfig.Version.Dependencies)
	if versionTrack.Extends != "" {
		parent, ok := atmosConfig.Version.Tracks[versionTrack.Extends]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTrackNotFound, versionTrack.Extends)
		}
		parentEntries, err := collectTrackEntries(atmosConfig, &parent)
		if err != nil {
			return nil, err
		}
		entries = parentEntries
	}
	mergeEntrySet(entries, versionTrack.Dependencies)
	return entries, nil
}

// mergeEntrySet overlays entries field-by-field so track dependencies can
// override only desired, policy, or metadata while inheriting the base catalog
// coordinate.
func mergeEntrySet(entries map[string]schema.VersionEntry, overlay map[string]schema.VersionEntry) {
	for name := range overlay {
		base := entries[name]
		override := overlay[name]
		entries[name] = mergeEntry(&base, &override)
	}
}

func mergeEntry(base, override *schema.VersionEntry) schema.VersionEntry {
	result := *base
	if override.Ecosystem != "" {
		result.Ecosystem = override.Ecosystem
	}
	if override.Datasource != "" {
		result.Datasource = override.Datasource
	}
	if override.Provider != "" {
		result.Provider = override.Provider
	}
	if override.Package != "" {
		result.Package = override.Package
	}
	if override.Desired != "" {
		result.Desired = override.Desired
	}
	if override.Group != "" {
		result.Group = override.Group
	}
	result.Update = mergeUpdatePolicy(result.Update, override.Update)
	if len(override.Include) > 0 {
		result.Include = append([]string{}, override.Include...)
	}
	result.Exclude = mergeStrings(result.Exclude, override.Exclude)
	if override.Prerelease != nil {
		result.Prerelease = boolPtr(*override.Prerelease)
	}
	if len(override.Labels) > 0 {
		result.Labels = append([]string{}, override.Labels...)
	}
	return result
}

// buildEffectiveEntry applies the policy inheritance chain to a single entry:
// global defaults, then track defaults, then the matched group policy, then the
// entry itself.
func buildEffectiveEntry(atmosConfig *schema.AtmosConfiguration, versionTrack *schema.VersionTrack, name string, entry *schema.VersionEntry) EffectiveEntry {
	effective := EffectiveEntry{
		Name:       name,
		Ecosystem:  canonicalEcosystem(entry.Ecosystem),
		Datasource: entry.Datasource,
		Provider:   entry.Provider,
		Package:    entry.Package,
		Desired:    entry.Desired,
		Group:      entry.Group,
		Update:     mergeUpdatePolicy(atmosConfig.Version.Defaults.Update, versionTrack.Defaults.Update, entry.Update),
		Include:    mostSpecificStrings(atmosConfig.Version.Defaults.Include, versionTrack.Defaults.Include),
		Exclude:    mergeStrings(atmosConfig.Version.Defaults.Exclude, versionTrack.Defaults.Exclude),
		Prerelease: effectivePrerelease(atmosConfig.Version.Defaults.Prerelease, versionTrack.Defaults.Prerelease),
		Labels:     mergeStrings(atmosConfig.Version.Defaults.Labels, versionTrack.Defaults.Labels, entry.Labels),
	}
	if effective.Package == "" {
		effective.Package = name
	}
	if effective.Datasource == "" {
		effective.Datasource = effective.Ecosystem
	}
	if effective.Group == "" {
		effective.Group = matchingGroup(atmosConfig.Version.Groups, name, &effective)
	}
	if group, ok := atmosConfig.Version.Groups[effective.Group]; ok && effective.Group != "" {
		effective.Update = mergeUpdatePolicy(effective.Update, group.Update)
		effective.Include = mostSpecificStrings(effective.Include, group.Include)
		effective.Exclude = mergeStrings(effective.Exclude, group.Exclude)
		if group.Prerelease != nil {
			effective.Prerelease = *group.Prerelease
		}
		effective.Labels = mergeStrings(effective.Labels, group.Labels)
	}
	effective.Include = mostSpecificStrings(effective.Include, entry.Include)
	effective.Exclude = mergeStrings(effective.Exclude, entry.Exclude)
	if entry.Prerelease != nil {
		effective.Prerelease = *entry.Prerelease
	}
	return effective
}

// matchingGroup returns the lexically first configured group whose match rules
// accept the entry, so group assignment stays deterministic.
func matchingGroup(groups map[string]schema.VersionGroup, name string, entry *EffectiveEntry) string {
	groupNames := make([]string, 0, len(groups))
	for groupName := range groups {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)
	for _, groupName := range groupNames {
		group := groups[groupName]
		if !matchesAnyEcosystem(group.Ecosystems, entry.Ecosystem) {
			continue
		}
		if !matchesAny(group.Datasources, entry.Datasource) {
			continue
		}
		if !matchesAny(group.Providers, entry.Provider) {
			continue
		}
		if matchesPattern(group.ExcludePatterns, name, entry.Package) {
			continue
		}
		if len(group.Patterns) == 0 || matchesPattern(group.Patterns, name, entry.Package) {
			return groupName
		}
	}
	return ""
}

func canonicalEcosystem(value string) string {
	switch value {
	case "github-actions":
		return "github/actions"
	default:
		return value
	}
}

func matchesAnyEcosystem(values []string, actual string) bool {
	if len(values) == 0 {
		return true
	}
	actual = canonicalEcosystem(actual)
	for _, value := range values {
		if canonicalEcosystem(value) == actual {
			return true
		}
	}
	return false
}

func matchesAny(values []string, actual string) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if value == actual {
			return true
		}
	}
	return false
}

func matchesPattern(patterns []string, name, pkg string) bool {
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
		if ok, _ := path.Match(pattern, pkg); ok {
			return true
		}
		if strings.Contains(name, pattern) || strings.Contains(pkg, pattern) {
			return true
		}
	}
	return false
}

// mergeUpdatePolicy merges update policies with last-writer-wins semantics for
// scalar fields and replacement for the schedule list.
func mergeUpdatePolicy(policies ...schema.VersionUpdatePolicy) schema.VersionUpdatePolicy {
	var result schema.VersionUpdatePolicy
	for _, policy := range policies {
		if policy.Strategy != "" {
			result.Strategy = policy.Strategy
		}
		if policy.Cooldown != "" {
			result.Cooldown = policy.Cooldown
		}
		if len(policy.Schedule) > 0 {
			result.Schedule = append([]string{}, policy.Schedule...)
		}
		if policy.Pin != "" {
			result.Pin = policy.Pin
		}
	}
	return result
}

// mergeStrings merges list fields with de-duplication, preserving order.
func mergeStrings(values ...[]string) []string {
	seen := map[string]bool{}
	var result []string
	for _, set := range values {
		for _, value := range set {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

// mostSpecificStrings returns the last non-empty list, preserving that scope's
// order and de-duplicating values within it.
func mostSpecificStrings(values ...[]string) []string {
	for i := len(values) - 1; i >= 0; i-- {
		if len(values[i]) > 0 {
			return mergeStrings(values[i])
		}
	}
	return nil
}

func effectivePrerelease(values ...*bool) bool {
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] != nil {
			return *values[i]
		}
	}
	return false
}

func boolPtr(value bool) *bool {
	return &value
}

// sortedNames returns the entry names in deterministic sorted order.
func sortedNames(entries map[string]EffectiveEntry) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
