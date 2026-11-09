package exec

import (
	"maps"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// applyUnitOverrides attaches a manifest's Terraform `overrides` to the units it imports, like the
// manifest's own components. Each unit section is copied first, so a provider's cached import result
// is never changed.
func applyUnitOverrides(fragments []map[string]any, overrides map[string]any) {
	if len(overrides) == 0 {
		return
	}
	for _, fragment := range fragments {
		section, _ := fragment[cfg.ComponentsSectionName].(map[string]any)
		terraform, _ := section[cfg.TerraformSectionName].(map[string]any)
		for name, component := range terraform {
			unit, ok := component.(map[string]any)
			if !ok {
				continue
			}
			unit = maps.Clone(unit)
			unit[cfg.OverridesSectionName] = overrides
			terraform[name] = unit
		}
	}
}
