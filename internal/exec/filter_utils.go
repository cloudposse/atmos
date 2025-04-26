package exec

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// FilterEmptySections recursively filters out empty sections and empty string values from a map
// based on the includeEmpty setting.
func FilterEmptySections(data map[string]any, includeEmpty bool) map[string]any {
	if includeEmpty {
		return data // No filtering needed
	}

	result := make(map[string]any)

	for key, value := range data {
		// Handle map values
		if mapValue, isMap := value.(map[string]any); isMap {
			// Check if map is empty
			if len(mapValue) == 0 {
				continue // Skip empty maps
			}

			// Check if all values are empty strings
			allEmptyStrings := true
			filteredMap := make(map[string]any)

			for subKey, subValue := range mapValue {
				if strValue, isStr := subValue.(string); isStr {
					if strValue != "" {
						allEmptyStrings = false
						filteredMap[subKey] = strValue
					}
				} else if subMapValue, isSubMap := subValue.(map[string]any); isSubMap {
					// Recursively filter nested maps
					filteredSubMap := FilterEmptySections(subMapValue, includeEmpty)
					if len(filteredSubMap) > 0 {
						allEmptyStrings = false
						filteredMap[subKey] = filteredSubMap
					}
				} else {
					// Non-string, non-map value
					allEmptyStrings = false
					filteredMap[subKey] = subValue
				}
			}

			// Skip if all values were empty strings
			if !allEmptyStrings {
				result[key] = filteredMap
			}
		} else if strValue, isStr := value.(string); isStr {
			// Skip empty string values
			if strValue != "" {
				result[key] = strValue
			}
		} else {
			// Keep non-string, non-map values
			result[key] = value
		}
	}

	return result
}

// GetIncludeEmptySetting gets the include_empty setting from the Atmos configuration
func GetIncludeEmptySetting(atmosConfig *schema.AtmosConfiguration) bool {
	// Default to true if setting is not provided
	includeEmpty := true
	if atmosConfig.Describe.Settings.IncludeEmpty != nil {
		includeEmpty = *atmosConfig.Describe.Settings.IncludeEmpty
	}
	return includeEmpty
}
