package list

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cloudposse/atmos/pkg/schema"
)

// getStatusIndicator returns a colored dot indicator based on enabled/locked state.
// - Gray (●) if enabled: false (disabled).
// - Red (●) if locked: true.
// - Green (●) if enabled: true and not locked.
func getStatusIndicator(enabled, locked bool) string {
	const statusDot = "●"

	switch {
	case locked:
		// Red for locked.
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(statusDot)
	case enabled:
		// Green for enabled.
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(statusDot)
	default:
		// Gray for disabled.
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(statusDot)
	}
}

// ExtractMetadata transforms schema.Instance slice into []map[string]any for renderer.
// Extracts metadata fields and makes them accessible to column templates.
func ExtractMetadata(instances []schema.Instance) []map[string]any {
	result := make([]map[string]any, 0, len(instances))

	for _, instance := range instances {
		// Get metadata fields with safe type assertions.
		var metadataType string
		var enabled, locked bool
		var componentVal, inherits, description string

		// Default type to "real" since abstract components are filtered out in createInstance.
		metadataType = "real"
		if val, ok := instance.Metadata["type"].(string); ok {
			metadataType = val
		}

		// Default enabled to true.
		enabled = true
		if val, ok := instance.Metadata["enabled"].(bool); ok {
			enabled = val
		}

		// Check locked in metadata.
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

		// Compute status indicator.
		status := getStatusIndicator(enabled, locked)

		// Determine the actual component folder used.
		// If metadata.component is set, use it (it's the base component).
		// Otherwise, use the component name itself.
		componentFolder := instance.Component
		if componentVal != "" {
			componentFolder = componentVal
		}

		// Create flat map with all fields accessible to templates.
		item := map[string]any{
			"status":           status, // Colored status dot (●)
			"stack":            instance.Stack,
			"component":        instance.Component,
			"component_type":   instance.ComponentType,
			"component_folder": componentFolder, // The actual component folder name
			"type":             metadataType,
			"enabled":          enabled,
			"locked":           locked,
			"component_base":   componentVal,
			"inherits":         inherits,
			"description":      description,
			"metadata":         instance.Metadata, // Full metadata for advanced filtering
			"vars":             instance.Vars,     // Expose vars for template access
			"settings":         instance.Settings, // Expose settings for template access
			"env":              instance.Env,      // Expose env for template access
		}

		result = append(result, item)
	}

	return result
}
