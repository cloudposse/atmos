package plandiff

import (
	"maps"
)

// normalizeResources copies resource records before removing excluded attributes.
// Identity, lifecycle actions, replacement paths, and non-attribute metadata remain intact.
func normalizeResources(resources map[string]interface{}, opts ...Option) map[string]interface{} {
	o := newOptions(opts...)
	result := maps.Clone(resources)
	for address, resource := range resources {
		record, ok := resource.(map[string]interface{})
		if !ok {
			continue
		}
		copied := maps.Clone(record)
		resourceType, _ := record["type"].(string)
		for _, field := range []string{"values", "sensitive_values"} {
			o.filterAttributeMap(copied, field, resourceType)
		}
		if change, ok := record["change"].(map[string]interface{}); ok {
			copiedChange := maps.Clone(change)
			for _, field := range []string{"before", "after", "after_unknown", "before_sensitive", "after_sensitive"} {
				o.filterAttributeMap(copiedChange, field, resourceType)
			}
			copied["change"] = copiedChange
		}
		result[address] = copied
	}
	return result
}

// filterAttributeMap only removes top-level resource attributes, never arbitrary nested keys.
func (o options) filterAttributeMap(record map[string]interface{}, field, resourceType string) {
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
