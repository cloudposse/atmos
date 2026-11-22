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

// BackendLayer is one component-level backend: its type and its settings.
type BackendLayer struct {
	Type     string
	Settings map[string]any
}

// CheckBackendOverrides rejects a component-level backend for a component a configuration provider
// owns: the unit's own backend is its state identity. A component without a provider is unaffected.
func CheckBackendOverrides(component, stack string, settings map[string]any, layers ...BackendLayer) error {
	defer perf.Track(nil, "configuration.CheckBackendOverrides")()

	// Only a provider owns the backend; a bare `configuration` key, such as an empty map, does not.
	if !OwnsComponent(map[string]any{"settings": settings}) {
		return nil
	}
	for _, layer := range layers {
		setting := ""
		if layer.Type != "" && layer.Type != "local" {
			setting = "backend_type"
		}
		if local, ok := layer.Settings["local"].(map[string]any); ok && len(local) > 0 {
			setting = "backend.local"
		}
		if setting != "" {
			return errUtils.Build(fmt.Errorf("%w: the state backend of component %q in stack %q is owned by the configuration provider, so `%s` cannot be set", errUtils.ErrConfigurationOverrideUnsupported, component, stack, setting)).
				WithContext("component", component).
				WithContext("stack", stack).
				WithHint("Define the backend in the unit's remote_state block; component backend overrides cannot change the unit's state identity.").Err()
		}
	}
	return nil
}
