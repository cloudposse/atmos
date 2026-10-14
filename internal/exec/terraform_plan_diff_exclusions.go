package exec

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// knownAfterApplyText is displayed for attribute values Terraform cannot determine until apply.
const knownAfterApplyText = "(known after apply)"

type planDiffOptions struct {
	ignoreChanges []string
	legacyDisplay bool
	// origUnknown and newUnknown hold top-level attributes reported in change.after_unknown
	// for the original and new plans. They affect display only, never the diff verdict.
	origUnknown map[string]bool
	newUnknown  map[string]bool
}

type planDiffOption func(*planDiffOptions)

// withPlanDiffIgnoreChanges adds validated resource_type.attribute exclusions.
func withPlanDiffIgnoreChanges(entries []string) planDiffOption {
	return func(options *planDiffOptions) {
		options.ignoreChanges = append(options.ignoreChanges, entries...)
	}
}

// withPlanDiffUnknownAttributes records the attributes that are unknown in each plan's resource.
func withPlanDiffUnknownAttributes(origUnknown, newUnknown map[string]bool) planDiffOption {
	return func(options *planDiffOptions) {
		options.origUnknown = origUnknown
		options.newUnknown = newUnknown
	}
}

// removedAttributeLine renders an attribute that is missing from the new plan. When the new
// plan reports it as unknown, the value is becoming known after apply rather than being removed.
func (o planDiffOptions) removedAttributeLine(attribute string, origValue interface{}) string {
	if o.newUnknown[attribute] {
		return fmt.Sprintf("  ~ %s: %v => %s\n", attribute, formatValue(origValue), knownAfterApplyText)
	}
	return fmt.Sprintf("  - %s: %v\n", attribute, formatValue(origValue))
}

// addedAttributeLine renders an attribute that is missing from the original plan. When the
// original plan reports it as unknown, the value was unknown and is now known.
func (o planDiffOptions) addedAttributeLine(attribute string, newValue interface{}) string {
	if o.origUnknown[attribute] {
		return fmt.Sprintf("  ~ %s: %s => %v\n", attribute, knownAfterApplyText, formatValue(newValue))
	}
	return fmt.Sprintf("  + %s: %v\n", attribute, formatValue(newValue))
}

// ignores matches configured exclusions exactly, without automatic attribute exclusions.
func (o planDiffOptions) ignores(resourceType, attribute string) bool {
	return slices.Contains(o.ignoreChanges, resourceType+"."+attribute)
}

// legacyHiddenAttribute preserves the formatter-only exclusions for older editions.
// These attributes still participate in comparison unless explicitly excluded.
func legacyHiddenAttribute(attribute string) bool {
	switch attribute {
	case "response_body_base64", "content_base64sha256", "content_base64sha512",
		"content_md5", "content_sha1", "content_sha256", "content_sha512":
		return true
	default:
		return false
	}
}

// planDiffPolicy resolves configuration before executing Terraform. Edition pins
// only select display behavior; explicit exclusions apply under every edition.
func planDiffPolicy(config *schema.AtmosConfiguration) (planDiffOption, error) {
	if err := config.Components.Terraform.PlanDiff.Validate(); err != nil {
		return nil, err
	}
	show, err := edition.BehaviorChanged(config.Edition, "terraform.plan_diff.show_computed_attributes")
	if err != nil {
		return nil, err
	}
	return func(options *planDiffOptions) {
		withPlanDiffIgnoreChanges(config.Components.Terraform.PlanDiff.IgnoreChanges)(options)
		options.legacyDisplay = !show
	}, nil
}

// normalizePlanDiffResources copies resource records before removing excluded attributes.
// Identity, lifecycle actions, replacement paths, and non-attribute metadata remain intact.
func normalizePlanDiffResources(resources map[string]interface{}, opts ...planDiffOption) map[string]interface{} {
	options := planDiffOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	result := maps.Clone(resources)
	for address, resource := range resources {
		record, ok := resource.(map[string]interface{})
		if !ok {
			continue
		}
		copied := maps.Clone(record)
		resourceType, _ := record["type"].(string)
		for _, field := range []string{"values", "sensitive_values"} {
			options.filterAttributeMap(copied, field, resourceType)
		}
		if change, ok := record["change"].(map[string]interface{}); ok {
			copiedChange := maps.Clone(change)
			for _, field := range []string{"before", "after", "after_unknown", "before_sensitive", "after_sensitive"} {
				options.filterAttributeMap(copiedChange, field, resourceType)
			}
			copied["change"] = copiedChange
		}
		result[address] = copied
	}
	return result
}

// filterAttributeMap only removes top-level resource attributes, never arbitrary nested keys.
func (o planDiffOptions) filterAttributeMap(record map[string]interface{}, field, resourceType string) {
	attributes, ok := record[field].(map[string]interface{})
	if !ok {
		return
	}
	filtered := maps.Clone(attributes)
	for attribute := range attributes {
		if o.ignores(resourceType, attribute) {
			delete(filtered, attribute)
		}
	}
	record[field] = filtered
}

// unmatchedPlanDiffIgnoreChanges returns configured resource_type.attribute entries that cannot match
// anything in the given resource maps even though resources of that type are present. Entries whose
// resource type is absent are not reported because the configuration is global across components.
// The result is deduplicated and keeps configuration order.
func unmatchedPlanDiffIgnoreChanges(entries []string, resourceMaps ...map[string]interface{}) []string {
	typeAttributes := planDiffAttributesByType(resourceMaps...)
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

// planDiffAttributesByType collects the top-level attribute names seen on resources of each type.
// Every section that normalization can filter is included so that an entry matching any of them counts.
func planDiffAttributesByType(resourceMaps ...map[string]interface{}) map[string]map[string]struct{} {
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
			collectPlanDiffAttributeNames(result[resourceType], record, "values", "sensitive_values")
			if change, ok := record["change"].(map[string]interface{}); ok {
				collectPlanDiffAttributeNames(result[resourceType], change,
					"before", "after", "after_unknown", "before_sensitive", "after_sensitive")
			}
		}
	}
	return result
}

// collectPlanDiffAttributeNames adds the keys of each named map field to the set.
func collectPlanDiffAttributeNames(names map[string]struct{}, record map[string]interface{}, fields ...string) {
	for _, field := range fields {
		attributes, _ := record[field].(map[string]interface{})
		for attribute := range attributes {
			names[attribute] = struct{}{}
		}
	}
}

// warnUnmatchedPlanDiffIgnoreChanges tells the user about ignore_changes entries that match no
// attribute on resources of that type present in either plan, which usually means a resource
// address or a typo was configured. It never affects the diff verdict.
func warnUnmatchedPlanDiffIgnoreChanges(entries []string, origPlan, newPlan map[string]interface{}) {
	if len(entries) == 0 {
		return
	}
	for _, entry := range unmatchedPlanDiffIgnoreChanges(entries, getResources(origPlan), getResources(newPlan)) {
		resourceType, _, _ := strings.Cut(entry, ".")
		ui.Warningf("plan_diff.ignore_changes entry %q matched no attribute on %s resources; entries are resource_type.attribute, not resource addresses", entry, resourceType)
	}
}
