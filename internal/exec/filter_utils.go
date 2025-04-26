package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// SectionFilter defines the interface for filtering map sections.
type SectionFilter interface {
	Filter(data map[string]any) map[string]any
}

// sectionFilter implements SectionFilter to remove empty sections.
type sectionFilter struct{}

// Filter removes empty sections and empty string values from a map.
func (f *sectionFilter) Filter(data map[string]any) map[string]any {
	result := make(map[string]any)

	for key, value := range data {
		if filteredValue := f.filterValue(value); filteredValue != nil {
			result[key] = filteredValue
		}
	}

	return result
}

// filterValue processes a single value, recursively filtering nested maps.
func (f *sectionFilter) filterValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if filteredMap := f.Filter(v); len(filteredMap) > 0 {
			return filteredMap
		}
		return nil
	case string:
		if v != "" {
			return v
		}
		return nil
	default:
		return value
	}
}

// The FilterEmptySections filters out empty sections and empty string values from a map.
func FilterEmptySections(data map[string]any, includeEmpty bool) map[string]any {
	if includeEmpty {
		return data
	}

	filter := &sectionFilter{}
	return filter.Filter(data)
}

// GetIncludeEmptySetting gets the include_empty setting from the Atmos configuration.
func GetIncludeEmptySetting(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig.Describe.Settings.IncludeEmpty != nil {
		return *atmosConfig.Describe.Settings.IncludeEmpty
	}
	return true
}
