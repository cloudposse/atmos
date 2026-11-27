package exec

import (
	"fmt"
	"maps"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

// Normalize a component's ordered input files before inheritance merges its
// manifest with others. Native -var-file replaces an entire variable value.
func normalizeVariableLayers(manifest map[string]any, file string) error {
	components, _ := manifest[cfg.ComponentsSectionName].(map[string]any)
	for _, kind := range []string{cfg.TerraformComponentType, cfg.PackerComponentType} {
		entries, _ := components[kind].(map[string]any)
		for name, raw := range entries {
			component, _ := raw.(map[string]any)
			layers, ok := component[cfg.VarsSectionName].([]any)
			if !ok {
				continue
			}
			vars := make(map[string]any)
			for i, layer := range layers {
				values, ok := layer.(map[string]any)
				if !ok {
					return fmt.Errorf("%w: %s: components.%s.%s.vars[%d] must resolve to a mapping", errUtils.ErrInvalidVarsSection, file, kind, name, i)
				}
				maps.Copy(vars, values)
			}
			component[cfg.VarsSectionName] = vars
		}
	}
	return nil
}
