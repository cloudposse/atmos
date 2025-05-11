package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	DefaultIncludeEmpty = true
)

// SectionFilter defines the interface for filtering map sections.
type SectionFilter interface {
	Filter(data map[string]any) map[string]any
}

type sectionFilter struct{}

func (f *sectionFilter) Filter(data map[string]any) map[string]any {
	result := make(map[string]any)

	for key, originalValue := range data {
		filteredValue := f.filterValue(originalValue)
		if filteredValue != nil || originalValue == nil {
			result[key] = filteredValue
		}
	}

	return result
}

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

func FilterEmptySections(data map[string]any, includeEmpty bool) map[string]any {
	if includeEmpty {
		return data
	}

	filter := &sectionFilter{}
	return filter.Filter(data)
}

// GetIncludeEmptySetting gets the include_empty setting from the Atmos configuration.
func GetIncludeEmptySetting(atmosConfig *schema.AtmosConfiguration) bool {
	if atmosConfig == nil || atmosConfig.Describe.Settings.IncludeEmpty == nil {
		return DefaultIncludeEmpty
	}
	return *atmosConfig.Describe.Settings.IncludeEmpty
}
