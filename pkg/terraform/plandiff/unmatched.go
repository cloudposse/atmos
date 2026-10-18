package plandiff

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// UnmatchedIgnoreChanges returns configured resource_type.attribute entries that cannot match
// anything in the two plans even though resources of that type are present in either plan.
// Entries whose resource type is absent are not reported because the configuration is global
// across components. The result is deduplicated and keeps configuration order.
func UnmatchedIgnoreChanges(entries []string, orig, fresh map[string]interface{}) []string {
	defer perf.Track(nil, "plandiff.UnmatchedIgnoreChanges")()

	return unmatchedIgnoreChanges(entries, getResources(orig), getResources(fresh))
}

// WarnUnmatchedIgnoreChanges tells the user about ignore_changes entries that match no
// attribute on resources of that type present in either plan, which usually means a resource
// address or a typo was configured. It never affects the diff verdict.
func WarnUnmatchedIgnoreChanges(entries []string, orig, fresh map[string]interface{}) {
	defer perf.Track(nil, "plandiff.WarnUnmatchedIgnoreChanges")()

	if len(entries) == 0 {
		return
	}
	for _, entry := range UnmatchedIgnoreChanges(entries, orig, fresh) {
		resourceType, _, _ := strings.Cut(entry, ".")
		ui.Warningf("plan_diff.ignore_changes entry %q matched no attribute on %s resources; entries are resource_type.attribute, not resource addresses", entry, resourceType)
	}
}

// unmatchedIgnoreChanges implements UnmatchedIgnoreChanges over already extracted resource maps.
func unmatchedIgnoreChanges(entries []string, resourceMaps ...map[string]interface{}) []string {
	typeAttributes := attributesByType(resourceMaps...)
	var unmatched []string
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, duplicate := seen[entry]; duplicate {
			continue
		}
		seen[entry] = struct{}{}
		resourceType, attribute, found := strings.Cut(entry, ".")
		if !found {
			continue
		}
		attributes, typePresent := typeAttributes[resourceType]
		if !typePresent {
			continue
		}
		if _, matched := attributes[attribute]; !matched {
			unmatched = append(unmatched, entry)
		}
	}
	return unmatched
}

// attributesByType collects the top-level attribute names seen on resources of each type.
// Every section that normalization can filter is included so that an entry matching any of them counts.
func attributesByType(resourceMaps ...map[string]interface{}) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{})
	for _, resources := range resourceMaps {
		for _, resource := range resources {
			record, ok := resource.(map[string]interface{})
			if !ok {
				continue
			}
			resourceType, _ := record["type"].(string)
			if resourceType == "" {
				continue
			}
			if result[resourceType] == nil {
				result[resourceType] = make(map[string]struct{})
			}
			collectAttributeNames(result[resourceType], record, "values", "sensitive_values")
			if change, ok := record["change"].(map[string]interface{}); ok {
				collectAttributeNames(result[resourceType], change,
					"before", "after", "after_unknown", "before_sensitive", "after_sensitive")
			}
		}
	}
	return result
}

// collectAttributeNames adds the keys of each named map field to the set.
func collectAttributeNames(names map[string]struct{}, record map[string]interface{}, fields ...string) {
	for _, field := range fields {
		attributes, _ := record[field].(map[string]interface{})
		for attribute := range attributes {
			names[attribute] = struct{}{}
		}
	}
}
