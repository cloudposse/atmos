// Package manager implements the Atmos Version Tracker: externally managed
// versions declared in atmos.yaml, resolved into a lock file, and consumed at
// runtime via the !version YAML function and the .version template context.
package manager

import (
	"errors"
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
	ErrTrackNotFound          = errors.New("version track not found")
	ErrVersionNotFound        = errors.New("version not found")
	ErrVersionNotLocked       = errors.New("version not locked")
	ErrTrackNotVerified       = errors.New("version track is not verified")
	ErrDesiredVersionRequired = errors.New("desired version is required")
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
	Allow      []string                   `yaml:"allow,omitempty" json:"allow,omitempty"`
	Ignore     []string                   `yaml:"ignore,omitempty" json:"ignore,omitempty"`
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

// TrackNames returns sorted configured track names.
func TrackNames(atmosConfig *schema.AtmosConfiguration) []string {
	defer perf.Track(atmosConfig, "manager.TrackNames")()

	names := make([]string, 0, len(atmosConfig.Version.Tracks))
	for name := range atmosConfig.Version.Tracks {
		names = append(names, name)
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
		return nil, fmt.Errorf("%w: %s", ErrTrackNotFound, track)
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

// collectTrackEntries merges a track's version entries over its extended parent.
func collectTrackEntries(atmosConfig *schema.AtmosConfiguration, versionTrack *schema.VersionTrack) (map[string]schema.VersionEntry, error) {
	entries := map[string]schema.VersionEntry{}
	if versionTrack.Extends != "" {
		parent, ok := atmosConfig.Version.Tracks[versionTrack.Extends]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrTrackNotFound, versionTrack.Extends)
		}
		for name := range parent.Versions {
			entries[name] = parent.Versions[name]
		}
	}
	for name := range versionTrack.Versions {
		entries[name] = versionTrack.Versions[name]
	}
	return entries, nil
}

// buildEffectiveEntry applies the policy inheritance chain to a single entry:
// global defaults, then track defaults, then the entry itself, then the
// matched group policy.
func buildEffectiveEntry(atmosConfig *schema.AtmosConfiguration, versionTrack *schema.VersionTrack, name string, entry *schema.VersionEntry) EffectiveEntry {
	effective := EffectiveEntry{
		Name:       name,
		Ecosystem:  entry.Ecosystem,
		Datasource: entry.Datasource,
		Provider:   entry.Provider,
		Package:    entry.Package,
		Desired:    entry.Desired,
		Group:      entry.Group,
		Update:     mergeUpdatePolicy(atmosConfig.Version.Defaults.Update, versionTrack.Defaults.Update, entry.Update),
		Allow:      mergeStrings(atmosConfig.Version.Defaults.Allow, versionTrack.Defaults.Allow, entry.Allow),
		Ignore:     mergeStrings(atmosConfig.Version.Defaults.Ignore, versionTrack.Defaults.Ignore, entry.Ignore),
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
		effective.Allow = mergeStrings(effective.Allow, group.Allow)
		effective.Ignore = mergeStrings(effective.Ignore, group.Ignore)
		effective.Labels = mergeStrings(effective.Labels, group.Labels)
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
		if !matchesAny(group.Ecosystems, entry.Ecosystem) {
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
		if policy.Automerge != nil {
			result.Automerge = policy.Automerge
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

// sortedNames returns the entry names in deterministic sorted order.
func sortedNames(entries map[string]EffectiveEntry) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
