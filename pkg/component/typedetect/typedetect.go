// Package typedetect decides which component type a component name belongs to when the caller
// did not name the type explicitly.
//
// Detection is presence-first: the caller supplies a Probe that reports, from the merged stack
// manifests alone, whether a component is defined under a given component type. Nothing is
// evaluated (no templates, no YAML functions, no cloud or auth calls) while deciding. The
// component is then processed exactly once, under the one type that defines it, so an error
// raised while evaluating that component (for example a missing producer referenced by a YAML
// function) is reported as-is instead of being mistaken for "this type does not have it".
package typedetect

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Probe reports whether the component is defined under componentType in the requested stack.
// It must only inspect already-merged stack manifests and must not evaluate templates or YAML
// functions.
type Probe func(componentType string) (bool, error)

// Resolve returns the single component type, among componentTypes, that defines the component.
//
// It returns an empty type and no error when no type defines the component, leaving the caller
// to report "component not found" in whatever form it already uses. It returns an error wrapping
// errUtils.ErrDuplicateComponentConfig when more than one type defines the component, and
// returns any error the probe reports.
func Resolve(component, stack string, componentTypes []string, probe Probe) (string, error) {
	defer perf.Track(nil, "typedetect.Resolve")()

	var present []string
	for _, componentType := range componentTypes {
		found, err := probe(componentType)
		if err != nil {
			return "", err
		}
		if found {
			present = append(present, componentType)
		}
	}

	switch len(present) {
	case 0:
		return "", nil
	case 1:
		return present[0], nil
	default:
		return "", errUtils.Build(fmt.Errorf("%w: the component `%s` is defined under more than one component type in the stack `%s`: %s",
			errUtils.ErrDuplicateComponentConfig, component, stack, strings.Join(present, ", "))).
			WithHint("Rename the component, or define it under only one `components.<type>` section.").
			WithContext("component", component).
			WithContext("stack", stack).
			WithContext("component_types", strings.Join(present, ", ")).
			Err()
	}
}
