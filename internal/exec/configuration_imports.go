package exec

import (
	"maps"

	"github.com/cloudposse/atmos/pkg/component/configuration"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// importConfigurationComponents routes configured provider schemes before YAML or
// remote-source resolution. The provider's units come back as a manifest fragment that
// joins the importing manifest, so they are components of the importing stack.
func importConfigurationComponents(config *schema.AtmosConfiguration, uri string, declaration *schema.StackImport, loadOptions *StackLoadOptions) (map[string]any, bool, error) {
	return configuration.ImportComponents(config, uri, declaration, loadOptions != nil && loadOptions.StaticOnly)
}

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
