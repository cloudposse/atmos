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

// instanceMetadata holds extracted metadata fields from a schema.Instance.
type instanceMetadata struct {
	metadataType    string
	enabled         bool
	locked          bool
	componentVal    string
	inherits        string
	description     string
	componentFolder string
	status          string
}

// ExtractMetadata transforms schema.Instance slice into []map[string]any for renderer.
// Extracts metadata fields and makes them accessible to column templates.
func ExtractMetadata(instances []schema.Instance) []map[string]any {
	result := make([]map[string]any, 0, len(instances))

	for i := range instances {
		metadata := extractInstanceMetadata(&instances[i])
		item := buildMetadataMap(&instances[i], metadata)
		result = append(result, item)
	}

	return result
}

// extractInstanceMetadata extracts and processes metadata fields from an instance.
func extractInstanceMetadata(instance *schema.Instance) instanceMetadata {
	metadata := instanceMetadata{
		metadataType: getMetadataType(instance),
		enabled:      getEnabledStatus(instance),
		locked:       getLockedStatus(instance),
		componentVal: getComponentValue(instance),
		inherits:     getInheritsString(instance),
		description:  getDescription(instance),
	}

	metadata.componentFolder = determineComponentFolder(instance.Component, metadata.componentVal)
	metadata.status = getStatusIndicator(metadata.enabled, metadata.locked)

	return metadata
}

// getMetadataType extracts the metadata type, defaulting to "real".
func getMetadataType(instance *schema.Instance) string {
	if val, ok := instance.Metadata["type"].(string); ok {
		return val
	}
	return "real"
}

// getEnabledStatus extracts the enabled status, defaulting to true.
func getEnabledStatus(instance *schema.Instance) bool {
	if val, ok := instance.Metadata[metadataEnabled].(bool); ok {
		return val
	}
	return true
}

// getLockedStatus extracts the locked status.
func getLockedStatus(instance *schema.Instance) bool {
	if val, ok := instance.Metadata[metadataLocked].(bool); ok {
		return val
	}
	return false
}

// getComponentValue extracts the component value from metadata.
func getComponentValue(instance *schema.Instance) string {
	if val, ok := instance.Metadata["component"].(string); ok {
		return val
	}
	return ""
}

// getInheritsString converts the inherits array to a comma-separated string.
func getInheritsString(instance *schema.Instance) string {
	val, ok := instance.Metadata["inherits"].([]interface{})
	if !ok {
		return ""
	}

	inheritsSlice := convertToStringSlice(val)
	return joinWithComma(inheritsSlice)
}

// convertToStringSlice converts []interface{} to []string.
func convertToStringSlice(values []interface{}) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

// joinWithComma joins a string slice with comma separators.
func joinWithComma(values []string) string {
	if len(values) == 0 {
		return ""
	}

	result := ""
	for i, s := range values {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// getDescription extracts the description from metadata.
func getDescription(instance *schema.Instance) string {
	if val, ok := instance.Metadata["description"].(string); ok {
		return val
	}
	return ""
}

// determineComponentFolder determines the actual component folder.
// If componentVal is set, use it (base component); otherwise use component name.
func determineComponentFolder(component, componentVal string) string {
	if componentVal != "" {
		return componentVal
	}
	return component
}

// buildMetadataMap creates a flat map with all fields accessible to templates.
func buildMetadataMap(instance *schema.Instance, metadata instanceMetadata) map[string]any {
	return map[string]any{
		"status":           metadata.status, // Colored status dot (●)
		"stack":            instance.Stack,
		"component":        instance.Component,
		"component_type":   instance.ComponentType,
		"component_folder": metadata.componentFolder, // The actual component folder name
		"type":             metadata.metadataType,
		"enabled":          metadata.enabled,
		"locked":           metadata.locked,
		"component_base":   metadata.componentVal,
		"inherits":         metadata.inherits,
		"description":      metadata.description,
		"metadata":         instance.Metadata, // Full metadata for advanced filtering
		"vars":             instance.Vars,     // Expose vars for template access
		"settings":         instance.Settings, // Expose settings for template access
		"env":              instance.Env,      // Expose env for template access
	}
}
