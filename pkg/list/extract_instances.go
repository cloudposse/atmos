package list

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExtractInstances transforms schema.Instance slice into []map[string]any for renderer.
// Converts structured Instance objects into flat maps accessible by column templates.
func ExtractInstances(instances []schema.Instance) []map[string]any {
	result := make([]map[string]any, 0, len(instances))

	for _, instance := range instances {
		// Create flat map with all instance fields accessible to templates.
		item := map[string]any{
			"component":      instance.Component,
			"stack":          instance.Stack,
			"component_type": instance.ComponentType,
			"vars":           instance.Vars,
			"settings":       instance.Settings,
			"env":            instance.Env,
			"backend":        instance.Backend,
			"metadata":       instance.Metadata,
		}

		result = append(result, item)
	}

	return result
}
