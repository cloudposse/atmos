package extract

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// maxDependentDepth is the maximum recursion depth for flattening dependents.
// This prevents potential stack overflow with extremely deep dependency chains.
const maxDependentDepth = 100

// Affected transforms a slice of schema.Affected into []map[string]any for the renderer.
// If flattenDependents is true, dependents are flattened into separate rows with is_dependent=true.
func Affected(affected []schema.Affected, flattenDependents bool) []map[string]any {
	defer perf.Track(nil, "extract.Affected")()

	result := make([]map[string]any, 0, len(affected))

	for i := range affected {
		item := affectedToMap(&affected[i], false, 0)
		result = append(result, item)

		// If flattening dependents, add them as separate rows.
		if flattenDependents && len(affected[i].Dependents) > 0 {
			depItems := flattenDependentsRecursive(affected[i].Dependents, 1)
			result = append(result, depItems...)
		}
	}

	return result
}

// affectedToMap converts a single Affected struct to map[string]any.
func affectedToMap(a *schema.Affected, isDependent bool, depth int) map[string]any {
	defer perf.Track(nil, "extract.affectedToMap")()

	// Get enabled/locked from settings if available.
	enabled := getEnabledFromSettings(a.Settings)
	locked := getLockedFromSettings(a.Settings)
	status := getStatusIndicator(enabled, locked)
	statusText := getStatusText(enabled, locked)

	item := map[string]any{
		"component":        a.Component,
		"component_type":   a.ComponentType,
		"component_path":   a.ComponentPath,
		"stack":            a.Stack,
		"stack_slug":       a.StackSlug,
		"namespace":        a.Namespace,
		"tenant":           a.Tenant,
		"environment":      a.Environment,
		"stage":            a.Stage,
		"affected":         a.Affected,
		"affected_all":     strings.Join(a.AffectedAll, ","),
		"file":             a.File,
		"folder":           a.Folder,
		"spacelift_stack":  a.SpaceliftStack,
		"atlantis_project": a.AtlantisProject,
		"is_dependent":     isDependent,
		"depth":            depth,
		"dependents_count": len(a.Dependents),
		"settings":         a.Settings,
		"enabled":          enabled,
		"locked":           locked,
		"status":           status,
		"status_text":      statusText,
	}

	return item
}

// flattenDependentsRecursive flattens nested dependents into a flat list.
// Recursion is limited to maxDependentDepth to prevent stack overflow.
func flattenDependentsRecursive(dependents []schema.Dependent, depth int) []map[string]any {
	defer perf.Track(nil, "extract.flattenDependentsRecursive")()

	result := make([]map[string]any, 0)

	for i := range dependents {
		item := dependentToMap(&dependents[i], depth)
		result = append(result, item)

		// Recurse for nested dependents if within depth limit.
		if len(dependents[i].Dependents) > 0 && depth < maxDependentDepth {
			nested := flattenDependentsRecursive(dependents[i].Dependents, depth+1)
			result = append(result, nested...)
		}
	}

	return result
}

// dependentToMap converts a Dependent struct to map[string]any.
func dependentToMap(d *schema.Dependent, depth int) map[string]any {
	defer perf.Track(nil, "extract.dependentToMap")()

	// Get enabled/locked from settings if available.
	enabled := getEnabledFromSettings(d.Settings)
	locked := getLockedFromSettings(d.Settings)
	status := getStatusIndicator(enabled, locked)
	statusText := getStatusText(enabled, locked)

	return map[string]any{
		"component":        d.Component,
		"component_type":   d.ComponentType,
		"component_path":   d.ComponentPath,
		"stack":            d.Stack,
		"stack_slug":       d.StackSlug,
		"namespace":        d.Namespace,
		"tenant":           d.Tenant,
		"environment":      d.Environment,
		"stage":            d.Stage,
		"affected":         "dependent",
		"affected_all":     "dependent",
		"file":             "",
		"folder":           "",
		"spacelift_stack":  d.SpaceliftStack,
		"atlantis_project": d.AtlantisProject,
		"is_dependent":     true,
		"depth":            depth,
		"dependents_count": len(d.Dependents),
		"settings":         d.Settings,
		"enabled":          enabled,
		"locked":           locked,
		"status":           status,
		"status_text":      statusText,
	}
}

// getEnabledFromSettings extracts the enabled status from settings, defaulting to true.
func getEnabledFromSettings(settings map[string]any) bool {
	if settings == nil {
		return true
	}

	// Check metadata.enabled in settings if available.
	if metadata, ok := settings["metadata"].(map[string]any); ok {
		if val, ok := metadata[metadataEnabled].(bool); ok {
			return val
		}
	}

	return true
}

// getLockedFromSettings extracts the locked status from settings, defaulting to false.
func getLockedFromSettings(settings map[string]any) bool {
	if settings == nil {
		return false
	}

	// Check metadata.locked in settings if available.
	if metadata, ok := settings["metadata"].(map[string]any); ok {
		if val, ok := metadata[metadataLocked].(bool); ok {
			return val
		}
	}

	return false
}
