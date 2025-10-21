package exec

import "github.com/cloudposse/atmos/pkg/schema"

// vendorSourceFilter provides methods for filtering vendor sources.
type vendorSourceFilter struct{}

// newVendorSourceFilter creates a new vendor source filter.
func newVendorSourceFilter() *vendorSourceFilter {
	return &vendorSourceFilter{}
}

// filterSources filters vendor sources based on component name and tags.
func filterSources(sources []schema.AtmosVendorSource, component string, tags []string) []schema.AtmosVendorSource {
	filter := newVendorSourceFilter()
	var filtered []schema.AtmosVendorSource

	for i := range sources {
		if filter.shouldIncludeSource(&sources[i], component, tags) {
			filtered = append(filtered, sources[i])
		}
	}

	return filtered
}

// shouldIncludeSource determines if a source should be included based on filters.
func (f *vendorSourceFilter) shouldIncludeSource(source *schema.AtmosVendorSource, component string, tags []string) bool {
	// Check component filter
	if !f.matchesComponent(source, component) {
		return false
	}

	// Check tags filter
	if !f.matchesTags(source, tags) {
		return false
	}

	return true
}

// matchesComponent checks if source matches the component filter.
func (f *vendorSourceFilter) matchesComponent(source *schema.AtmosVendorSource, component string) bool {
	// If no component filter specified, include all
	if component == "" {
		return true
	}
	// Check if source component matches filter
	return source.Component == component
}

// matchesTags checks if source has at least one matching tag.
func (f *vendorSourceFilter) matchesTags(source *schema.AtmosVendorSource, tags []string) bool {
	// If no tags filter specified, include all
	if len(tags) == 0 {
		return true
	}

	// Check if source has any matching tag
	return f.hasAnyMatchingTag(source.Tags, tags)
}

// hasAnyMatchingTag checks if any source tag matches the filter tags.
func (f *vendorSourceFilter) hasAnyMatchingTag(sourceTags []string, filterTags []string) bool {
	for _, filterTag := range filterTags {
		if f.containsTag(sourceTags, filterTag) {
			return true
		}
	}
	return false
}

// containsTag checks if a tag exists in the tag list.
func (f *vendorSourceFilter) containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
