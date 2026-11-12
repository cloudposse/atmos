package configuration

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ImportFragment wraps imported units as a stack manifest fragment, so the ordinary
// import merge adds them to the importing manifest as Terraform components.
func ImportFragment(components map[string]any) map[string]any {
	defer perf.Track(nil, "configuration.ImportFragment")()

	return map[string]any{"components": map[string]any{"terraform": components}}
}

// unitSource returns the source file of a provider-owned component, or "" for a YAML component.
func unitSource(component any) string {
	section, _ := component.(map[string]any)
	settings, _ := section["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	source, _ := configuration["source"].(string)
	return source
}

// CheckImportConflicts rejects a manifest in which two different units become one component
// name. A YAML component of the same name as a unit is not a conflict: the unit's fragment is
// the base layer of the ordinary import merge, so the YAML component deep-merges over (or under,
// when it comes from an earlier import) the unit like any other stack configuration. Configs are
// the manifest's imports in order, followed by the manifest's own content.
func CheckImportConflicts(manifest string, configs []map[string]any) error {
	defer perf.Track(nil, "configuration.CheckImportConflicts")()

	units := map[string]string{}
	for _, config := range configs {
		section, _ := config["components"].(map[string]any)
		terraform, _ := section["terraform"].(map[string]any)
		for name, component := range terraform {
			source := unitSource(component)
			if source == "" {
				continue
			}
			if previous, seen := units[name]; seen && previous != source {
				return ImportConflict(name, previous, source, manifest)
			}
			units[name] = source
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

// ImportConflict reports two units that would become one component of the importing stack.
func ImportConflict(component, first, second, where string) error {
	defer perf.Track(nil, "configuration.ImportConflict")()

	return errUtils.Build(fmt.Errorf("%w: units %s and %s both become component %q (%s)", errUtils.ErrConfigurationImportConflict, first, second, component, where)).
		WithContext("component", component).
		WithContext("unit", first).
		WithContext("other_unit", second).
		WithHint("Import the units from different stack files, or import a narrower directory or a single `terragrunt.hcl`").
		Err()
}
