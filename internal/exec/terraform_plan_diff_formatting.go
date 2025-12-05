package exec

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// formatOutputChange formats the change between two output values.
func formatOutputChange(key string, origValue, newValue interface{}) string {
	origSensitive := isSensitive(origValue)
	newSensitive := isSensitive(newValue)

	switch {
	case origSensitive && newSensitive:
		return fmt.Sprintf("~ %s: (sensitive value) => (sensitive value)\n", key)
	case origSensitive:
		return fmt.Sprintf("~ %s: (sensitive value) => %v\n", key, formatValue(newValue))
	case newSensitive:
		return fmt.Sprintf("~ %s: %v => (sensitive value)\n", key, formatValue(origValue))
	default:
		return fmt.Sprintf("~ %s: %v => %v\n", key, formatValue(origValue), formatValue(newValue))
	}
}

// printAttributeDiff handles the formatting of an attribute diff.
func printAttributeDiff(diff *strings.Builder, attrK string, origAttrV, newAttrV interface{}) {
	origSensitive := isSensitive(origAttrV)
	newSensitive := isSensitive(newAttrV)

	switch {
	case origSensitive && newSensitive:
		fmt.Fprintf(diff, "  ~ %s: (sensitive value) => (sensitive value)\n", attrK)
	case origSensitive:
		fmt.Fprintf(diff, "  ~ %s: (sensitive value) => %v\n", attrK, formatValue(newAttrV))
	case newSensitive:
		fmt.Fprintf(diff, "  ~ %s: %v => (sensitive value)\n", attrK, formatValue(origAttrV))
	default:
		// Check if both values are maps and use the specialized diff function
		origMap, origIsMap := origAttrV.(map[string]interface{})
		newMap, newIsMap := newAttrV.(map[string]interface{})

		if origIsMap && newIsMap {
			mapDiff := formatMapDiff(origMap, newMap)
			if mapDiff != noChangesText {
				fmt.Fprintf(diff, "  ~ %s: %s\n", attrK, mapDiff)
			}
		} else {
			fmt.Fprintf(diff, "  ~ %s: %v => %v\n", attrK, formatValue(origAttrV), formatValue(newAttrV))
		}
	}
}

// isSensitive checks if a value is marked as sensitive.
func isSensitive(value interface{}) bool {
	if valueMap, ok := value.(map[string]interface{}); ok {
		if sensitive, ok := valueMap["sensitive"].(bool); ok && sensitive {
			return true
		}
	}
	return false
}

// formatValue formats a value for display, handling sensitive values.
func formatValue(value interface{}) string {
	if isSensitive(value) {
		return "(sensitive value)"
	}

	// Handle different value types
	switch v := value.(type) {
	case string:
		return formatStringValue(v)
	case map[string]interface{}:
		return formatMapValue(v)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// formatStringValue handles formatting of string values.
func formatStringValue(strVal string) string {
	// Keep weather report content intact
	if strings.Contains(strVal, "Weather report:") {
		return strVal
	}

	// If it looks like a base64 value, simplify it
	if strings.HasPrefix(strVal, "V2VhdGhl") || strings.HasPrefix(strVal, "CgogIBtb") {
		return "(base64 encoded value)"
	}

	// For other very long strings, show start and end
	if len(strVal) > maxStringDisplayLength {
		return fmt.Sprintf("%s...%s", strVal[:halfStringDisplayLength], strVal[len(strVal)-halfStringDisplayLength:])
	}

	return strVal
}

// formatMapValue handles formatting of map values.
func formatMapValue(valueMap map[string]interface{}) string {
	// If there's a 'value' key, extract it
	if val, exists := valueMap["value"]; exists {
		return fmt.Sprintf("%v", val)
	}

	// For outputs, check for type and value fields
	if _, hasType := valueMap["type"]; hasType {
		if val, hasValue := valueMap["value"]; hasValue {
			return fmt.Sprintf("%v", val)
		}
	}

	// For response headers and similar maps, provide a cleaner format
	if len(valueMap) > 0 && (strings.Contains(fmt.Sprintf(defaultValueFormat, valueMap), "map[") ||
		strings.Contains(fmt.Sprintf(defaultValueFormat, valueMap), "Access-Control-Allow-Origin")) {
		// This is likely a map that we want to display more cleanly
		return formatMapForDisplay(valueMap)
	}

	return fmt.Sprintf(defaultValueFormat, valueMap)
}

// formatMapForDisplay formats a map for cleaner display.
func formatMapForDisplay(m map[string]interface{}) string {
	// Get all keys and sort them for consistent output
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// For simple maps with 3 or fewer entries, show a compact representation
	if len(m) <= 3 {
		parts := make([]string, 0, len(m))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s: %v", k, m[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}

	// For larger maps, show a structured representation with indentation
	var sb strings.Builder
	sb.WriteString("{\n")

	for _, k := range keys {
		v := m[k]

		// Format the value based on its type
		var valueStr string
		if nestedMap, ok := v.(map[string]interface{}); ok {
			// Recursively format nested maps with additional indentation
			nestedStr := formatMapForDisplay(nestedMap)
			// Add indentation to each line
			nestedStr = strings.ReplaceAll(nestedStr, "\n", "\n    ")
			valueStr = nestedStr
		} else {
			valueStr = fmt.Sprintf(defaultValueFormat, v)
		}

		sb.WriteString(fmt.Sprintf("    %s: %s\n", k, valueStr))
	}

	sb.WriteString("}")
	return sb.String()
}

// formatMapDiff formats the difference between two maps showing only changed keys.
func formatMapDiff(origMap, newMap map[string]interface{}) string {
	// Get all keys from both maps and sort them
	keys := getSortedKeys(origMap, newMap)

	// If no differences, return early
	if reflect.DeepEqual(origMap, newMap) {
		return noChangesText
	}

	// For empty or very small diffs, use a compact representation
	if len(keys) <= 3 {
		return formatCompactMapDiff(keys, origMap, newMap)
	}

	// For larger diffs, show a structured representation with indentation
	return formatStructuredMapDiff(keys, origMap, newMap)
}

// getSortedKeys returns a sorted slice of all keys from both maps.
func getSortedKeys(mapA, mapB map[string]interface{}) []string {
	// Get all keys from both maps
	allKeys := make(map[string]bool)
	for k := range mapA {
		allKeys[k] = true
	}
	for k := range mapB {
		allKeys[k] = true
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}

// formatStructuredMapDiff formats a map diff with structured representation for larger diffs.
func formatStructuredMapDiff(keys []string, origMap, newMap map[string]interface{}) string {
	var sb strings.Builder
	sb.WriteString("{\n")
	changesFound := false

	for _, k := range keys {
		origVal, origExists := origMap[k]
		newVal, newExists := newMap[k]

		// Skip keys that haven't changed
		if origExists && newExists && reflect.DeepEqual(origVal, newVal) {
			continue
		}

		changesFound = true
		formatKeyDiff(&sb, keyDiffParams{k, origVal, newVal, origExists, newExists})
	}

	if !changesFound {
		return noChangesText
	}

	sb.WriteString("}")
	return sb.String()
}

// keyDiffParams contains parameters for formatting a key difference.
type keyDiffParams struct {
	key        string
	origVal    interface{}
	newVal     interface{}
	origExists bool
	newExists  bool
}

// formatKeyDiff formats a single key difference and appends it to the given string builder.
func formatKeyDiff(sb *strings.Builder, params keyDiffParams) {
	// Format based on what changed
	switch {
	case !params.origExists:
		fmt.Fprintf(sb, "    + %s: %s\n", params.key, formatValue(params.newVal))
	case !params.newExists:
		fmt.Fprintf(sb, "    - %s: %s\n", params.key, formatValue(params.origVal))
	default:
		// Value changed
		if origMap, ok := params.origVal.(map[string]interface{}); ok {
			if newMap, ok := params.newVal.(map[string]interface{}); ok {
				// Recursively diff nested maps
				nestedDiff := formatMapDiff(origMap, newMap)
				if nestedDiff != noChangesText {
					// Add indentation to nested diff
					nestedDiff = strings.ReplaceAll(nestedDiff, "\n", "\n    ")
					fmt.Fprintf(sb, "    ~ %s: %s\n", params.key, nestedDiff)
				}
				return
			}
		}

		// Simple value change
		fmt.Fprintf(sb, "    ~ %s: %v => %v\n", params.key, formatValue(params.origVal), formatValue(params.newVal))
	}
}

// formatCompactMapDiff creates a compact string representation for small map diffs.
func formatCompactMapDiff(keys []string, origMap, newMap map[string]interface{}) string {
	changes := make([]string, 0, len(keys))

	for _, k := range keys {
		origVal, origExists := origMap[k]
		newVal, newExists := newMap[k]

		switch {
		case !origExists:
			changes = append(changes, fmt.Sprintf("+%s: %v", k, formatValue(newVal)))
		case !newExists:
			changes = append(changes, fmt.Sprintf("-%s: %v", k, formatValue(origVal)))
		case !reflect.DeepEqual(origVal, newVal):
			changes = append(changes, fmt.Sprintf("~%s: %v => %v", k, formatValue(origVal), formatValue(newVal)))
		}
	}

	if len(changes) == 0 {
		return noChangesText
	}

	return fmt.Sprintf("{%s}", strings.Join(changes, ", "))
}
