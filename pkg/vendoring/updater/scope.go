package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

const componentSelectionHashLength = 12

// FilterGroupComponents narrows report to the components matching include/exclude glob patterns,
// keeping only components whose latest update actually changed something.
func FilterGroupComponents(report *vendoring.UpdateReport, include, exclude []string) []string {
	defer perf.Track(nil, "updater.FilterGroupComponents")()

	components := make([]string, 0)
	seen := map[string]bool{}
	for _, result := range report.Results {
		if result.Status != vendoring.StatusUpdated || seen[result.Component] || !MatchesPatterns(result.Component, include, true) || MatchesPatterns(result.Component, exclude, false) {
			continue
		}
		seen[result.Component] = true
		components = append(components, result.Component)
	}
	sort.Strings(components)
	return components
}

// MatchesPatterns reports whether value matches any glob in patterns. Empty is returned when
// patterns is itself empty, so callers can distinguish an unconfigured include list (matches
// everything) from an unconfigured exclude list (matches nothing).
func MatchesPatterns(value string, patterns []string, empty bool) bool {
	defer perf.Track(nil, "updater.MatchesPatterns")()

	if len(patterns) == 0 {
		return empty
	}
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, value); ok {
			return true
		}
	}
	return false
}

// FilterReport narrows report to only the results for components.
func FilterReport(report *vendoring.UpdateReport, components []string) *vendoring.UpdateReport {
	defer perf.Track(nil, "updater.FilterReport")()

	allowed := map[string]bool{}
	for _, component := range components {
		allowed[component] = true
	}
	result := &vendoring.UpdateReport{}
	for _, row := range report.Results {
		if allowed[row.Component] {
			result.Results = append(result.Results, row)
		}
	}
	return result
}

// UpdateScope derives a stable, filesystem/branch-safe scope name from a group name or an explicit
// component selection, used to name PR branches and CI summaries consistently across runs.
func UpdateScope(group string, components []string) string {
	defer perf.Track(nil, "updater.UpdateScope")()

	if group != "" {
		return "group-" + group
	}
	if len(components) == 0 {
		return "all"
	}
	if len(components) == 1 {
		return "components-" + components[0]
	}
	copied := append([]string(nil), components...)
	sort.Strings(copied)
	hash := sha256.Sum256([]byte(strings.Join(copied, "\x00")))
	return "components-" + hex.EncodeToString(hash[:])[:componentSelectionHashLength]
}
