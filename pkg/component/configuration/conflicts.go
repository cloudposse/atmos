package configuration

import (
	"fmt"
	"maps"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ImportFragment wraps imported units as a stack manifest fragment, so the ordinary
// import merge adds them to the importing manifest as Terraform components.
func ImportFragment(components map[string]any) map[string]any {
	defer perf.Track(nil, "configuration.ImportFragment")()

	return map[string]any{"components": map[string]any{"terraform": components}}
}

// ApplyUnitOverrides attaches a manifest's Terraform `overrides` to the units it imports, like the
// manifest's own components. Each unit section is copied first, so a provider's cached import result
// is never changed.
func ApplyUnitOverrides(fragments []map[string]any, overrides map[string]any) {
	defer perf.Track(nil, "configuration.ApplyUnitOverrides")()

	if len(overrides) == 0 {
		return
	}
	for _, fragment := range fragments {
		section, _ := fragment["components"].(map[string]any)
		terraform, _ := section["terraform"].(map[string]any)
		for name, component := range terraform {
			unit, ok := component.(map[string]any)
			if !ok {
				continue
			}
			unit = maps.Clone(unit)
			unit["overrides"] = overrides
			terraform[name] = unit
		}
	}
}

// unitSource reads the source identity of an unmerged provider-owned component.
func unitSource(component any) string {
	section, _ := component.(map[string]any)
	settings, _ := section["settings"].(map[string]any)
	configuration, _ := settings["configuration"].(map[string]any)
	source, _ := configuration["source"].(string)
	return source
}

// ImportUnitSources extracts identity from a provider import fragment before YAML merging.
// Callers must not pass ordinary YAML or merged stack configuration.
func ImportUnitSources(fragment map[string]any) map[string]string {
	defer perf.Track(nil, "configuration.ImportUnitSources")()

	units := map[string]string{}
	section, _ := fragment["components"].(map[string]any)
	terraform, _ := section["terraform"].(map[string]any)
	for name, component := range terraform {
		if source := unitSource(component); source != "" {
			units[name] = source
		}
	}
	return units
}

// MergeImportUnitSources accumulates provider provenance across recursive imports.
func MergeImportUnitSources(manifest string, units, incoming map[string]string) error {
	defer perf.Track(nil, "configuration.MergeImportUnitSources")()

	for name, source := range incoming {
		if previous, seen := units[name]; seen && previous != source {
			return ImportConflict(name, previous, source, manifest)
		}
		units[name] = source
	}
	return nil
}

// CheckImportConflicts checks unmerged provider fragments for duplicate unit names.
// Ordinary YAML and merged stack configurations must not be passed here.
func CheckImportConflicts(manifest string, configs []map[string]any) error {
	defer perf.Track(nil, "configuration.CheckImportConflicts")()
	units := map[string]string{}
	for _, config := range configs {
		if err := MergeImportUnitSources(manifest, units, ImportUnitSources(config)); err != nil {
			return err
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
