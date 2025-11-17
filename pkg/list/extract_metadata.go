package list

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExtractMetadata transforms schema.Instance slice into []map[string]any for renderer.
// Extracts metadata fields and makes them accessible to column templates.
func ExtractMetadata(instances []schema.Instance) []map[string]any {
	result := make([]map[string]any, 0, len(instances))

	for _, instance := range instances {
		// Get metadata fields with safe type assertions.
		var metadataType string
		var enabled, locked bool
		var componentVal, inherits, description string

		if val, ok := instance.Metadata["type"].(string); ok {
			metadataType = val
		}

		if val, ok := instance.Metadata["enabled"].(bool); ok {
			enabled = val
		}

		if val, ok := instance.Metadata["locked"].(bool); ok {
			locked = val
		}

		if val, ok := instance.Metadata["component"].(string); ok {
			componentVal = val
		}

		if val, ok := instance.Metadata["inherits"].([]interface{}); ok {
			// Convert []interface{} to comma-separated string.
			inheritsSlice := make([]string, 0, len(val))
			for _, v := range val {
				if str, ok := v.(string); ok {
					inheritsSlice = append(inheritsSlice, str)
				}
			}
			if len(inheritsSlice) > 0 {
				for i, s := range inheritsSlice {
					if i > 0 {
						inherits += ", "
					}
					inherits += s
				}
			}
		}

		if val, ok := instance.Metadata["description"].(string); ok {
			description = val
		}

		// Create flat map with all fields accessible to templates.
		item := map[string]any{
			"stack":          instance.Stack,
			"component":      instance.Component,
			"component_type": instance.ComponentType,
			"type":           metadataType,
			"enabled":        enabled,
			"locked":         locked,
			"component_base": componentVal,
			"inherits":       inherits,
			"description":    description,
			"metadata":       instance.Metadata, // Full metadata for advanced filtering
		}

		result = append(result, item)
	}

	return result
}
