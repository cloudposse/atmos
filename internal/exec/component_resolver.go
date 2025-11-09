package exec

import (
	comp "github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// componentResolver is a package-level resolver instance that uses the exec stack loader.
var componentResolver *comp.Resolver

func init() {
	componentResolver = comp.NewResolver(NewStackLoader())
}

// ResolveComponentFromPath resolves a filesystem path to a component name and validates it exists in the stack.
// This is a wrapper around pkg/component.Resolver.ResolveComponentFromPath for backwards compatibility.
func ResolveComponentFromPath(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	stack string,
	expectedComponentType string,
) (string, error) {
	return componentResolver.ResolveComponentFromPath(atmosConfig, path, stack, expectedComponentType)
}

// ResolveComponentFromPathWithoutTypeCheck resolves a filesystem path to a component name without validating component type.
// This is a wrapper around pkg/component.Resolver.ResolveComponentFromPathWithoutTypeCheck for backwards compatibility.
func ResolveComponentFromPathWithoutTypeCheck(
	atmosConfig *schema.AtmosConfiguration,
	path string,
	stack string,
) (string, error) {
	return componentResolver.ResolveComponentFromPathWithoutTypeCheck(atmosConfig, path, stack)
}
