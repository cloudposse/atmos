package exec

import (
	"fmt"
	"maps"
	"slices"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/hooks/events"
)

// validateStackHookEvents checks only hook-bearing stack scopes, never arbitrary
// maps under vars/settings. Imports have already merged at this point. Checking
// declarations before component filtering also catches misspellings in previews.
func validateStackHookEvents(pin string, stack map[string]any) error {
	strict, err := edition.BehaviorChanged(pin, "hooks.registered_events")
	if err != nil || !strict {
		return err
	}
	if err := validateHookScope(pin, "stack", stack); err != nil {
		return err
	}
	for _, kind := range []string{"terraform", "helmfile", "packer", "ansible", "helm", "kubernetes", "aws/cloudformation"} {
		scope, _ := stack[kind].(map[string]any)
		if err := validateHookScope(pin, kind, scope); err != nil {
			return err
		}
	}
	components, _ := stack["components"].(map[string]any)
	for _, kind := range slices.Sorted(maps.Keys(components)) {
		instances, _ := components[kind].(map[string]any)
		for _, name := range slices.Sorted(maps.Keys(instances)) {
			scope, _ := instances[name].(map[string]any)
			if err := validateHookScope(pin, "components."+kind+"."+name, scope); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateHookScope(pin, path string, scope map[string]any) error {
	if section, exists := scope["hooks"]; exists {
		if err := events.ValidateSection(pin, section); err != nil {
			return fmt.Errorf("%s.hooks: %w", path, err)
		}
	}
	if overrides, ok := scope["overrides"].(map[string]any); ok {
		if err := events.ValidateSection(pin, overrides["hooks"]); err != nil {
			return fmt.Errorf("%s.overrides.hooks: %w", path, err)
		}
	}
	return nil
}
