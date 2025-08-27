//go:build !linting
// +build !linting

package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTemplateValues parses template values from command line arguments
// Format: key=value,key2=value2
func ParseTemplateValues(templateValues []string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, templateValue := range templateValues {
		// Check for multiple equals signs first
		if strings.Count(templateValue, "=") != 1 {
			return nil, fmt.Errorf("invalid template value format: %s (expected key=value)", templateValue)
		}

		// Split on equals sign
		parts := strings.SplitN(templateValue, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid template value format: %s (expected key=value)", templateValue)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check for empty key (including whitespace-only keys)
		if key == "" {
			return nil, fmt.Errorf("empty key in template value: %s", templateValue)
		}

		// Try to parse as different types
		parsedValue, err := ParseValue(value)
		if err != nil {
			return nil, fmt.Errorf("failed to parse value for key '%s': %w", key, err)
		}

		result[key] = parsedValue
	}

	return result, nil
}

// ParseValue attempts to parse a string value into the most appropriate type
func ParseValue(value string) (interface{}, error) {
	// Try to parse as boolean first
	switch strings.ToLower(value) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	}

	// Try to parse as number
	if strings.Contains(value, ".") {
		// Try float
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f, nil
		}
	} else {
		// Try int
		if i, err := strconv.Atoi(value); err == nil {
			return i, nil
		}
	}

	// If all else fails, treat as string
	return value, nil
}
