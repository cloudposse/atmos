package exec

import (
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

type planDiffOptions struct {
	ignoreChanges []string
	legacyDisplay bool
}

type planDiffOption func(*planDiffOptions)

// withPlanDiffIgnoreChanges adds validated resource_type.attribute exclusions.
func withPlanDiffIgnoreChanges(entries []string) planDiffOption {
	return func(options *planDiffOptions) {
		options.ignoreChanges = append(options.ignoreChanges, entries...)
	}
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
