package configuration

import (
	"fmt"
	"maps"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ImportFragment wraps imported units as a stack manifest fragment, so the ordinary
// import merge adds them to the importing manifest as Terraform components.
func ImportFragment(components map[string]any) map[string]any {
	defer perf.Track(nil, "configuration.ImportFragment")()

	return map[string]any{componentsKey: map[string]any{terraformType: components}}
}

// unitSource returns the source file of a provider-owned component, or "" for a YAML component.
func unitSource(component any) string {
	section, _ := component.(map[string]any)
	settings, _ := section["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	source, _ := configuration["source"].(string)
	return source
}

// CheckImportConflicts rejects a manifest in which one component name has two definitions:
// two different units, or a unit and a YAML component. Units own their configuration
// (inputs, backend, module source), so a YAML component of the same name would be
// merged over it silently or ignored. Configs are the manifest's imports in order,
// followed by the manifest's own content.
func CheckImportConflicts(manifest string, configs []map[string]any) error {
	defer perf.Track(nil, "configuration.CheckImportConflicts")()

	units := map[string]string{}
	yaml := map[string]bool{}
	for _, config := range configs {
		section, _ := config[componentsKey].(map[string]any)
		terraform, _ := section[terraformType].(map[string]any)
		for name, component := range terraform {
			source := unitSource(component)
			if source == "" {
				yaml[name] = true
				continue
			}
			if previous, seen := units[name]; seen && previous != source {
				return importConflict(name, previous, source, manifest)
			}
			units[name] = source
		}
	}
	for _, name := range slices.Sorted(maps.Keys(units)) {
		source := units[name]
		if yaml[name] {
			return errUtils.Build(fmt.Errorf("%w: component %q is defined by the Terragrunt unit %s and also by a YAML component (manifest %s)", errUtils.ErrConfigurationImportConflict, name, source, manifest)).
				WithContext("component", name).
				WithContext("unit", source).
				WithContext("manifest", manifest).
				WithHint("A unit owns its inputs, backend and module; remove the YAML component, or derive a new one with `metadata.inherits: [" + name + "]` under another name").
				Err()
		}
	}
	return nil
}

// OwnsComponent reports whether a configuration provider supplies a component section, so
// callers can avoid attributing it to unrelated YAML files.
func OwnsComponent(component map[string]any) bool {
	defer perf.Track(nil, "configuration.OwnsComponent")()

	settings, _ := component["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	return configuration["provider"] != nil
}
