package create

import (
	"fmt"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// declaredComponent is one validated --component kind:name declaration.
type declaredComponent struct {
	kind, name string
}

// parseComponents validates --component values. Kind and name are trimmed, a name cannot contain
// whitespace, and duplicates are detected after trimming so "terraform:a " and "terraform:a" collide.
func parseComponents(config *schema.AtmosConfiguration, declarations []string) ([]declaredComponent, error) {
	parsed := make([]declaredComponent, 0, len(declarations))
	seen := make(map[declaredComponent]bool, len(declarations))
	for _, value := range declarations {
		kind, name, ok := strings.Cut(value, ":")
		kind, name = strings.TrimSpace(kind), strings.TrimSpace(name)
		if !ok || name == "" || !knownKind(kind, config.Commands) {
			return nil, fmt.Errorf("%w: component %q must use a supported kind:name", errUtils.ErrInvalidFlag, value)
		}
		if strings.ContainsFunc(name, unicode.IsSpace) {
			return nil, fmt.Errorf("%w: component name %q cannot contain whitespace", errUtils.ErrInvalidFlag, name)
		}
		component := declaredComponent{kind: kind, name: name}
		if seen[component] {
			return nil, fmt.Errorf("%w: duplicate component %q", errUtils.ErrInvalidFlag, kind+":"+name)
		}
		seen[component] = true
		parsed = append(parsed, component)
	}
	return parsed, nil
}

// componentStubs renders disabled component entries for the template's .Config.stack_components.
func componentStubs(declared []declaredComponent) map[string]map[string]any {
	components := make(map[string]map[string]any)
	for _, component := range declared {
		if components[component.kind] == nil {
			components[component.kind] = make(map[string]any)
		}
		components[component.kind][component.name] = map[string]any{"metadata": map[string]any{"enabled": false}, "vars": map[string]any{}}
	}
	return components
}
